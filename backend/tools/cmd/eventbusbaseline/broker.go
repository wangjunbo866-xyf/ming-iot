// 本文件通过真实 broker 位点与消费结果验证完成边界，不使用时间等待代替提交证据。
package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	contract "gitee.com/ming-iot/share/contracts/eventbus"
	foundation "gitee.com/ming-iot/share/foundation/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

// waitCommitted 比较组提交位点与调用时锁定的三个分区末尾。
func waitCommitted(ctx context.Context, group string) error {
	cl, err := kgo.NewClient(kgo.SeedBrokers("kafka:29092"))
	if err != nil {
		return err
	}
	defer cl.Close()
	ends := kmsg.NewPtrListOffsetsRequest()
	ends.ReplicaID = -1
	ends.IsolationLevel = 1
	topic := kmsg.NewListOffsetsRequestTopic()
	topic.Topic = contract.CoordinationTopic
	for i := int32(0); i < 3; i++ {
		p := kmsg.NewListOffsetsRequestTopicPartition()
		p.Partition = i
		p.Timestamp = -1
		topic.Partitions = append(topic.Partitions, p)
	}
	ends.Topics = []kmsg.ListOffsetsRequestTopic{topic}
	end, err := ends.RequestWith(ctx, cl)
	if err != nil {
		return err
	}
	target := map[int32]int64{}
	for _, t := range end.Topics {
		for _, p := range t.Partitions {
			if p.ErrorCode != 0 {
				return fmt.Errorf("end offset error %d", p.ErrorCode)
			}
			target[p.Partition] = p.Offset
		}
	}
	if len(target) != 3 {
		return fmt.Errorf("wanted three partition offsets, got %v", target)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		fetch := kmsg.NewPtrOffsetFetchRequest()
		fetch.Group = group
		fetch.Topics = []kmsg.OffsetFetchRequestTopic{{Topic: contract.CoordinationTopic, Partitions: []int32{0, 1, 2}}}
		fetch.Groups = []kmsg.OffsetFetchRequestGroup{{Group: group, Topics: []kmsg.OffsetFetchRequestGroupTopic{{Topic: contract.CoordinationTopic, Partitions: []int32{0, 1, 2}}}}}
		r, err := fetch.RequestWith(ctx, cl)
		if err != nil {
			return err
		}
		got := map[int32]int64{}
		if r.ErrorCode == 0 {
			for _, t := range r.Topics {
				for _, p := range t.Partitions {
					if p.ErrorCode == 0 {
						got[p.Partition] = p.Offset
					}
				}
			}
		}
		for _, g := range r.Groups {
			if g.ErrorCode == 0 {
				for _, t := range g.Topics {
					for _, p := range t.Partitions {
						if p.ErrorCode == 0 {
							got[p.Partition] = p.Offset
						}
					}
				}
			}
		}
		ok := true
		for p, offset := range target {
			if offset > 0 && got[p] < offset {
				ok = false
			}
		}
		if ok {
			return nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return fmt.Errorf("commit wait: %w target=%v got=%v", ctx.Err(), target, got)
		}
	}
}

// competition 验证相同逻辑组的两个真实消费者分配分区，而不是每个副本重复处理。
func competition(ctx context.Context) error {
	run := fmt.Sprintf("run-%d", time.Now().UnixNano())
	group, err := contract.CompetingGroup(run)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	counts := map[string]int{}
	owners := map[int]int{}
	parts := map[int]map[int32]bool{0: {}, 1: {}}
	consumers := make([]*foundation.BusConsumer, 0, 2)
	done := make(chan error, 2)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// start 启动同一逻辑身份的新副本，重新入组仍由 broker 决定分配。
	start := func(index int, suffix string) (*foundation.BusConsumer, error) {
		c, err := foundation.NewBusConsumer(foundation.BusConsumerConfig{Client: clientConfig(fmt.Sprintf("%s-%d-%s", run, index, suffix)), GroupID: group, Topic: contract.CoordinationTopic, PollTimeout: 200 * time.Millisecond, RequestTimeout: 2 * time.Second, MaxPollRecords: 32})
		if err != nil {
			return nil, err
		}
		go func() {
			done <- c.Run(runCtx, func(_ context.Context, r *kgo.Record) error {
				e, err := contract.Decode(r.Value)
				if err != nil {
					return err
				}
				p, err := e.CacheInvalidation()
				if err != nil {
					return err
				}
				if p.CacheType != "p1-competition" || !strings.HasPrefix(p.Key, run) {
					return nil
				}
				mu.Lock()
				counts[p.Key]++
				owners[index]++
				parts[index][r.Partition] = true
				mu.Unlock()
				return nil
			})
		}()
		return c, nil
	}
	for i := 0; i < 2; i++ {
		c, err := start(i, "initial")
		if err != nil {
			return err
		}
		consumers = append(consumers, c)
		defer c.Close()
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, a := consumers[0].CacheState()
		_, b := consumers[1].CacheState()
		if a && b {
			break
		}
		select {
		case <-ticker.C:
		case err := <-done:
			return fmt.Errorf("consumer stopped early: %v", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	producer, err := newBus("publisher-" + run)
	if err != nil {
		return err
	}
	defer producer.Close()
	for i := 0; i < 40; i++ {
		if err := producer.Publish(ctx, "server.cache.sync.p1-competition", fmt.Sprintf("%s-%02d", run, i)); err != nil {
			return err
		}
	}
	for {
		mu.Lock()
		n := len(counts)
		mu.Unlock()
		if n == 40 {
			break
		}
		select {
		case <-ticker.C:
		case err := <-done:
			return fmt.Errorf("consumer stopped: %v", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err = waitCommitted(ctx, group); err != nil {
		return err
	}
	// 先让一个成员退出，验证保留分区的成员不会永久丢失 ready。
	survivorGeneration, _ := consumers[0].CacheState()
	if err = consumers[1].Close(); err != nil {
		return err
	}
	<-done
	for {
		generation, ready := consumers[0].CacheState()
		if ready && generation > survivorGeneration {
			break
		}
		select {
		case <-ticker.C:
		case err := <-done:
			return fmt.Errorf("survivor stopped after peer left: %v", err)
		case <-ctx.Done():
			return fmt.Errorf("survivor readiness after peer left: %w", ctx.Err())
		}
	}
	// 新成员加入触发协作式增量分配，随后用新消息验证消费仍向前推进。
	consumers[1], err = start(1, "rejoined")
	if err != nil {
		return err
	}
	defer consumers[1].Close()
	for {
		_, a := consumers[0].CacheState()
		_, b := consumers[1].CacheState()
		if a && b {
			break
		}
		select {
		case <-ticker.C:
		case err := <-done:
			return fmt.Errorf("consumer stopped on rejoin: %v", err)
		case <-ctx.Done():
			return fmt.Errorf("rejoin readiness: %w", ctx.Err())
		}
	}
	for i := 40; i < 60; i++ {
		if err := producer.Publish(ctx, "server.cache.sync.p1-competition", fmt.Sprintf("%s-%02d", run, i)); err != nil {
			return err
		}
	}
	for {
		mu.Lock()
		n := len(counts)
		mu.Unlock()
		if n == 60 {
			break
		}
		select {
		case <-ticker.C:
		case err := <-done:
			return fmt.Errorf("consumer stopped after rejoin: %v", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err = waitCommitted(ctx, group); err != nil {
		return err
	}
	cancel()
	for _, c := range consumers {
		_ = c.Close()
	}
	for range consumers {
		<-done
	}
	mu.Lock()
	defer mu.Unlock()
	for k, n := range counts {
		if n != 1 {
			return fmt.Errorf("duplicate %s count=%d", k, n)
		}
	}
	if owners[0] == 0 || owners[1] == 0 {
		return fmt.Errorf("both replicas must receive partitions: %v", owners)
	}
	fmt.Printf("{\"case\":\"same-group-competition\",\"result\":\"PASS\",\"messages\":60,\"member_leave_rejoin\":true,\"replica_a\":%d,\"replica_b\":%d}\n", owners[0], owners[1])
	return nil
}
