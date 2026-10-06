# versionedkv

Go 实现的并发安全、带单调递增版本号的内存 KV Store。仅使用标准库，无第三方依赖。

## 特性

- 每个非空 Key 对应一个 Value 和一个 uint64 Version，首次 Put 版本为 1
- 每次成功修改（Put / CompareAndSwap / Delete）该 Key 的版本号 +1，严格单调递增
- Delete 保留墓碑版本：Key 删除后重新 Put 不会从 1 重启，也不会复用旧版本
- CAS 的检查与更新在同一把写锁内完成，是原子操作；并发下基于同一旧版本的 CAS 最多只有一个成功
- Get / Snapshot 返回副本；Snapshot 按 Key 字典序稳定排列，只包含当前存在的 Key

## 接口

| 方法 | 说明 |
| --- | --- |
| `New() *Store` | 创建空 Store，可安全并发使用 |
| `Put(key, value string) (uint64, error)` | 写入并返回新版本号 |
| `Get(key string) (string, uint64, bool)` | 读取 Value 和 Version；Key 不存在时 ok=false |
| `CompareAndSwap(key string, expectedVersion uint64, newValue string) (uint64, error)` | 版本匹配时原子更新并返回新版本号 |
| `Delete(key string) (bool, error)` | 删除并保留版本历史；返回是否删掉了存在的 Key |
| `Snapshot() []Item` | 返回按 Key 排序的快照副本（`Item{Key, Value, Version}`） |
| `Len() int` | 当前存在的 Key 数量 |

错误为哨兵错误，可用 `errors.Is` 判断：

- `ErrEmptyKey`：Key 为空字符串
- `ErrKeyNotFound`：CAS 时 Key 不存在（含已删除）
- `ErrVersionMismatch`：CAS 时期望版本与当前版本不一致

## 版本语义

- `Put`：该 Key 最新版本 +1（首次为 1）
- `CompareAndSwap` 成功：当前版本 +1
- `Delete`：当前版本 +1 并标记墓碑；之后重新 `Put` 再 +1（例：Put→1，Delete→2，再 Put→3）
- 不同 Key 的版本互相独立

## 运行

演示程序（内置数据演示 Put / Get / 成功与失败 CAS / Delete / 重新 Put）：

```sh
go run ./cmd/demo
```

测试（含竞态检测）：

```sh
go test -race ./...
```
