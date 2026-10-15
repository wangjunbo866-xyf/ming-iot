# 历史片段归属

本目录记录历史文档中的摘录、改写和设计参考；原历史正文保留。以下固定版本是对照来源，不冒认当时的 checkout，不覆盖全文其他材料的权利范围，也不为 ming-iot 整仓授予新许可。

## NATS

Source: https://github.com/nats-io/nats-server/tree/c1ca29ba0d040c259bcf23e1a7451c38609ab14a

许可原文：[NATS](licenses/NATS-LICENSE.txt)。对应材料及摘要见 [sources.json](sources.json)。

```text
// Copyright 2012-2026 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
```

另有 v2.10.26 (`14fa949d74bcb07a8cce684a59ce39262bd4b00b`) 对照，许可文本摘要相同；原文混合版本背景尚不构成唯一来源身份。

## nbio

Source: https://github.com/lesismal/nbio/tree/8ad01e6461af31d510d27b679f670c3dddbe417c

许可原文：[nbio](licenses/nbio-LICENSE.txt)。对应材料及摘要见 [sources.json](sources.json)。

## comqtt

Source: https://github.com/wind-c/comqtt/tree/c68325655488ddc2d9f798c218a68e88fd08d913

许可原文：[comqtt](licenses/comqtt-LICENSE.txt)。对应材料及摘要见 [sources.json](sources.json)。

packets 原头部：

```text
// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co
```

短 Put 函数为格式调整后的代码对应；PublishDecode 为历史简化示意；同文 byteslice 片段来源另列 gnet。

## gnet

Source: https://github.com/panjf2000/gnet/tree/fd7ce4bef97582920c174b56a96cbbda7a5bcd42

许可原文：[gnet](licenses/gnet-LICENSE.txt)。对应材料及摘要见 [sources.json](sources.json)。

保留固定源码的 The Gnet Authors 归属：Copyright (c) 2019 The Gnet Authors；Copyright (c) 2021 The Gnet Authors。对应 byteslice 摘录与走查中的局部对应均不代表整份文档原样来自 gnet。

## go-zero

Source: https://github.com/zeromicro/go-zero/tree/0f2b589d4db9e7db2251c1a95fae867493058bb3

许可原文：[go-zero](licenses/go-zero-LICENSE.txt)。对应材料及摘要见 [sources.json](sources.json)。

文档明确参考 v1.8.4 配置模型，字段标签改为 YAML，另有新增与省略字段；不是上游声明原样复制，也不承诺配置语义等价。当前 Things 候选依赖为 v1.9.4，勿将历史版本描述作当前依赖事实。

## Go GC 文字

来源：[A Guide to the Go Garbage Collector](https://go.dev/doc/gc-guide)，Go 项目；固定网站源码版本 `4e6db4a174674104d39a69481e2b6deb01f3df49`。历史文档第二句为改写，应按转述阅读。网站文字适用条款见 [Go copyright](https://go.dev/copyright) 和 [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)；代码另按其 BSD 条款，不混用。

上述归属和原许可保留不能代替最终发布范围、其他第三方材料及运行时义务核对。
