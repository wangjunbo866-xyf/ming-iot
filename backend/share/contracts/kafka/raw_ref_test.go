// 本文件验证 Kafka 原始记录引用的标识、Topic、数字格式与确定性编码契约。
package kafka

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

const (
	validRawRefSourceClusterID = "AAECAwQFBgcICQoLDA0ODw"
	validRawRefTopicID         = "EBESExQVFhcYGRobHB0eHw"
	validRawRefTopic           = "iot.data.raw.v1.modbus-tcp"
)

// TestRawRefAcceptsValidValueAndRoundTrips 验证合法 RawRef 的构造、文本格式、编码和 getters。
func TestRawRefAcceptsValidValueAndRoundTrips(t *testing.T) {
	t.Parallel()

	ref, err := NewRawRef(validRawRefSourceClusterID, validRawRefTopicID, 3, 42)
	if err != nil {
		t.Fatalf("NewRawRef() error = %v", err)
	}
	wantText := validRawRefSourceClusterID + "/" + validRawRefTopicID + "/3/42"
	if got := ref.String(); got != wantText {
		t.Fatalf("RawRef.String() = %q, want %q", got, wantText)
	}
	if got := ref.Encode(); !bytes.Equal(got, []byte(wantText)) {
		t.Fatalf("RawRef.Encode() = %q, want %q", got, wantText)
	}
	parsed, err := ParseRawRef(ref.String())
	if err != nil {
		t.Fatalf("ParseRawRef() error = %v", err)
	}
	decoded, err := DecodeRawRef(ref.Encode())
	if err != nil {
		t.Fatalf("DecodeRawRef() error = %v", err)
	}
	if parsed.String() != wantText || decoded.String() != wantText {
		t.Fatalf("RawRef round-trip mismatch: parsed=%q decoded=%q", parsed, decoded)
	}
	if got := ref.SourceClusterID(); got != validRawRefSourceClusterID {
		t.Fatalf("SourceClusterID() = %q", got)
	}
	if got := ref.TopicID(); got != validRawRefTopicID {
		t.Fatalf("TopicID() = %q", got)
	}
	if got := ref.Partition(); got != 3 {
		t.Fatalf("Partition() = %d", got)
	}
	if got := ref.Offset(); got != 42 {
		t.Fatalf("Offset() = %d", got)
	}
}

// TestRawRefAcceptsZeroNumbers 验证分区和 offset 的最小非负值可使用规范十进制表示。
func TestRawRefAcceptsZeroNumbers(t *testing.T) {
	t.Parallel()

	ref, err := NewRawRef(validRawRefSourceClusterID, validRawRefTopicID, 0, 0)
	if err != nil {
		t.Fatalf("NewRawRef() error = %v", err)
	}
	if got, want := ref.String(), validRawRefSourceClusterID+"/"+validRawRefTopicID+"/0/0"; got != want {
		t.Fatalf("RawRef.String() = %q, want %q", got, want)
	}
}

// TestRawRefZeroValueUsesEmptyRepresentations 验证零值不会生成可误认成引用的文本或字节。
func TestRawRefZeroValueUsesEmptyRepresentations(t *testing.T) {
	t.Parallel()

	var ref RawRef
	if got := ref.String(); got != "" {
		t.Fatalf("RawRef{}.String() = %q, want empty", got)
	}
	if got := ref.Encode(); got != nil {
		t.Fatalf("RawRef{}.Encode() = %q, want nil", got)
	}
}

// TestRawRefRejectsInvalidConstructorInputs 验证 ID、Topic、分区和 offset 的非法输入失败关闭。
func TestRawRefRejectsInvalidConstructorInputs(t *testing.T) {
	t.Parallel()

	validIDBytes := make([]byte, 16)
	tests := []struct {
		name          string
		sourceCluster string
		topicID       string
		partition     int32
		offset        int64
	}{
		{name: "source ID 长度错误", sourceCluster: "short", topicID: validRawRefTopicID},
		{name: "source ID 使用填充", sourceCluster: validRawRefSourceClusterID + "=", topicID: validRawRefTopicID},
		{name: "source ID 非法字符", sourceCluster: strings.Repeat("!", 22), topicID: validRawRefTopicID},
		{name: "source ID 非规范尾部", sourceCluster: "AAAAAAAAAAAAAAAAAAAAAB", topicID: validRawRefTopicID},
		{name: "topic ID 长度错误", sourceCluster: validRawRefSourceClusterID, topicID: "short"},
		{name: "topic ID 使用标准 base64", sourceCluster: validRawRefSourceClusterID, topicID: base64.StdEncoding.EncodeToString(validIDBytes)},
		{name: "空 Topic ID", sourceCluster: validRawRefSourceClusterID, topicID: ""},
		{name: "Topic ID 含斜杠", sourceCluster: validRawRefSourceClusterID, topicID: "AA/"},
		{name: "Topic ID 含空白", sourceCluster: validRawRefSourceID(), topicID: "AA "},
		{name: "Topic ID 含非法字符", sourceCluster: validRawRefSourceClusterID, topicID: "AA+"},
		{name: "Topic ID 点号保留名", sourceCluster: validRawRefSourceClusterID, topicID: "."},
		{name: "负分区", sourceCluster: validRawRefSourceClusterID, topicID: validRawRefTopicID, partition: -1},
		{name: "负 offset", sourceCluster: validRawRefSourceClusterID, topicID: validRawRefTopicID, offset: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewRawRef(test.sourceCluster, test.topicID, test.partition, test.offset); err == nil {
				t.Fatalf("NewRawRef() error = nil, want rejection")
			}
		})
	}
}

// TestRawRefRejectsNonCanonicalText 验证 ParseRawRef 严格要求四段、规范数字和无空白文本。
func TestRawRefRejectsNonCanonicalText(t *testing.T) {
	t.Parallel()

	base := validRawRefSourceClusterID + "/" + validRawRefTopicID
	tests := []string{
		"",
		base,
		base + "/1",
		base + "/1/2/3",
		base + "//1/2",
		base + "/01/2",
		base + "/+1/2",
		base + "/-1/2",
		base + "/1/+2",
		base + "/1/-2",
		base + "/ 1/2",
		base + "/1/2 ",
		base + "/2147483648/0",
		base + "/0/9223372036854775808",
		" " + base + "/1/2",
		base + "/1/2\n",
	}
	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			if _, err := ParseRawRef(text); err == nil {
				t.Fatalf("ParseRawRef(%q) error = nil, want rejection", text)
			}
		})
	}
}

// TestRawRefRejectsOversizedTextBeforeSegmentation 验证超过最大 canonical 长度的输入优先失败。
func TestRawRefRejectsOversizedTextBeforeSegmentation(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("x", 77)
	_, err := ParseRawRef(text)
	if err == nil {
		t.Fatal("ParseRawRef() error = nil, want length rejection")
	}
	for _, want := range []string{"actual=77", "limit=76"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ParseRawRef() error = %q, want containing %q", err, want)
		}
	}
}

// TestRawRefRejectsEqualLengthInvalidBase64IDs 验证 22 字符非法 ID 命中编码校验而非长度校验。
func TestRawRefRejectsEqualLengthInvalidBase64IDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "source cluster ID",
			text: strings.Repeat("!", 22) + "/" + validRawRefTopicID + "/0/0",
			want: "invalid source cluster ID",
		},
		{
			name: "topic ID",
			text: validRawRefSourceClusterID + "/" + strings.Repeat("!", 22) + "/0/0",
			want: "invalid topic ID",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseRawRef(test.text)
			if err == nil {
				t.Fatal("ParseRawRef() error = nil, want base64 rejection")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParseRawRef() error = %q, want containing %q", err, test.want)
			}
		})
	}
}

// TestRawRefDecodeRejectsNonCanonicalBytes 验证 DecodeRawRef 不接受尾随字节或不可解析文本。
func TestRawRefDecodeRejectsNonCanonicalBytes(t *testing.T) {
	t.Parallel()

	valid := []byte(validRawRefSourceClusterID + "/" + validRawRefTopicID + "/3/42")
	for _, encoded := range [][]byte{
		append(append([]byte(nil), valid...), 0x00),
		[]byte("\x00" + string(valid)),
		[]byte("invalid"),
	} {
		if _, err := DecodeRawRef(encoded); err == nil {
			t.Fatalf("DecodeRawRef(%q) error = nil, want rejection", encoded)
		}
	}
}

// TestRawRefEncodeResultIsIndependent 验证 Encode 返回新 slice，不暴露内部状态。
func TestRawRefEncodeResultIsIndependent(t *testing.T) {
	t.Parallel()

	ref, err := NewRawRef(validRawRefSourceClusterID, validRawRefTopicID, 1, 2)
	if err != nil {
		t.Fatalf("NewRawRef() error = %v", err)
	}
	encoded := ref.Encode()
	encoded[0] = 'X'
	if got, want := string(ref.Encode()), ref.String(); got != want {
		t.Fatalf("RawRef changed after Encode() result mutation: got=%q want=%q", got, want)
	}
}

// validRawRefSourceID 返回一个规范的 16 字节 RawRef ID。
func validRawRefSourceID() string {
	return validRawRefSourceClusterID
}
