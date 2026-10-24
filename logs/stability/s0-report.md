# S0 冒烟：.85(经 dae) vs Mac(经主路由 .1)

| 探针 | 主机 | 轮数 | 层 |
|---|---|---|---|
| a85 | Debian-te | 58 | l0,l1,l2,l3,l4 |
| mac | sthhodeMac-mini.local | 60 | l1,l2,l3,l4 |

## 分层聚合（失败率 / p50 / p95 / max）

| 探针 | 层 | 目标 | 成功 | 失败 | 失败率 | p50 | p95 | max | 主要错误 |
|---|---|---|---|---|---|---|---|---|---|
| a85 | l0 | cn-ali | 58 | 0 | 0.0% | 7.7ms | 8.5ms | 9.2ms |  |
| a85 | l0 | cn-baidu | 58 | 0 | 0.0% | 7.7ms | 8.7ms | 9.0ms |  |
| a85 | l0 | global-1111 | 54 | 4 | 6.9% | 213.0ms | 236.0ms | 244.0ms |  |
| a85 | l0 | global-cf | 51 | 7 | 12.1% | 154.0ms | 160.0ms | 162.0ms |  |
| a85 | l0 | global-google | 0 | 58 | 100.0% | - | - | - |  |
| a85 | l0 | global-ytb | 0 | 58 | 100.0% | - | - | - |  |
| a85 | l0 | sub-link | 9 | 0 | 0.0% | 163.0ms | 166.0ms | 166.0ms |  |
| a85 | l0 | sub-s3 | 8 | 1 | 11.1% | 194.0ms | 195.0ms | 195.0ms |  |
| a85 | l1 | cn-ali | 58 | 0 | 0.0% | 7.8ms | 8.8ms | 8.9ms |  |
| a85 | l1 | cn-baidu | 58 | 0 | 0.0% | 8.8ms | 11.5ms | 18.0ms |  |
| a85 | l1 | global-1111 | 58 | 0 | 0.0% | 0.3ms | 0.9ms | 4.7ms |  |
| a85 | l1 | global-cf | 58 | 0 | 0.0% | 1.2ms | 3.3ms | 34.1ms |  |
| a85 | l1 | global-google | 58 | 0 | 0.0% | 1.1ms | 5.7ms | 24.5ms |  |
| a85 | l1 | global-ytb | 58 | 0 | 0.0% | 1.0ms | 4.4ms | 8.2ms |  |
| a85 | l1 | sub-link | 9 | 0 | 0.0% | 1.0ms | 1.3ms | 1.3ms |  |
| a85 | l1 | sub-s3 | 9 | 0 | 0.0% | 1.3ms | 26.6ms | 26.6ms |  |
| a85 | l2 | cn-ali | 58 | 0 | 0.0% | 20.9ms | 28.2ms | 33.8ms |  |
| a85 | l2 | cn-baidu | 58 | 0 | 0.0% | 33.1ms | 41.6ms | 44.4ms |  |
| a85 | l2 | global-1111 | 58 | 0 | 0.0% | 673.2ms | 2001.1ms | 2732.4ms |  |
| a85 | l2 | global-cf | 58 | 0 | 0.0% | 762.4ms | 2819.2ms | 3708.0ms |  |
| a85 | l2 | global-google | 58 | 0 | 0.0% | 751.6ms | 2166.0ms | 2723.4ms |  |
| a85 | l2 | global-ytb | 58 | 0 | 0.0% | 860.7ms | 2263.3ms | 5151.7ms |  |
| a85 | l2 | sub-link | 9 | 0 | 0.0% | 983.9ms | 1524.9ms | 1524.9ms |  |
| a85 | l2 | sub-s3 | 9 | 0 | 0.0% | 942.7ms | 2080.0ms | 2080.0ms |  |
| a85 | l3 | cn-ali | 0 | 58 | 100.0% | - | - | - |  |
| a85 | l3 | cn-baidu | 58 | 0 | 0.0% | - | - | - |  |
| a85 | l3 | global-1111 | 8 | 50 | 86.2% | - | - | - | TimeoutError |
| a85 | l3 | global-cf | 11 | 47 | 81.0% | - | - | - | TimeoutError |
| a85 | l3 | global-google | 7 | 51 | 87.9% | - | - | - | TimeoutError |
| a85 | l3 | global-ytb | 35 | 23 | 39.7% | - | - | - | TimeoutError |
| a85 | l3 | sub-link | 0 | 9 | 100.0% | - | - | - |  |
| a85 | l3 | sub-s3 | 0 | 9 | 100.0% | - | - | - | TimeoutError |
| a85 | l4 | dns:www.baidu.com | 116 | 58 | 33.3% | 0.9ms | 2.9ms | 6.6ms | TimeoutError |
| a85 | l4 | dns:www.cloudflare.com | 116 | 58 | 33.3% | 0.5ms | 1.0ms | 2.7ms | TimeoutError |
| a85 | l4 | dns:www.youtube.com | 116 | 58 | 33.3% | 0.4ms | 0.9ms | 1.3ms | TimeoutError |
| mac | l1 | cn-ali | 60 | 0 | 0.0% | 10.5ms | 12.1ms | 28.8ms |  |
| mac | l1 | cn-baidu | 60 | 0 | 0.0% | 14.8ms | 30.3ms | 57.3ms |  |
| mac | l1 | global-1111 | 0 | 60 | 100.0% | - | - | - | timeout |
| mac | l1 | global-cf | 0 | 60 | 100.0% | - | - | - | timeout |
| mac | l1 | global-google | 0 | 60 | 100.0% | - | - | - | timeout |
| mac | l1 | global-ytb | 0 | 60 | 100.0% | - | - | - | timeout |
| mac | l1 | sub-link | 0 | 10 | 100.0% | - | - | - | timeout |
| mac | l1 | sub-s3 | 0 | 10 | 100.0% | - | - | - | timeout |
| mac | l2 | cn-ali | 60 | 0 | 0.0% | 35.5ms | 40.7ms | 79.6ms |  |
| mac | l2 | cn-baidu | 60 | 0 | 0.0% | 43.0ms | 89.5ms | 116.3ms |  |
| mac | l3 | cn-ali | 0 | 60 | 100.0% | - | - | - |  |
| mac | l3 | cn-baidu | 60 | 0 | 0.0% | - | - | - |  |
| mac | l4 | dns:www.baidu.com | 120 | 60 | 33.3% | 8.9ms | 11.1ms | 19.6ms | timeout |
| mac | l4 | dns:www.cloudflare.com | 120 | 60 | 33.3% | 9.2ms | 11.3ms | 12.4ms | timeout |
| mac | l4 | dns:www.youtube.com | 120 | 60 | 33.3% | 9.2ms | 11.3ms | 13.7ms | timeout |

## 断流事件（同一目标连续 ≥3 次失败，基于 l3）

| 探针 | 目标 | 开始(UTC) | 持续 | 失败次数 | 未恢复 |
|---|---|---|---|---|---|
| a85 | cn-ali | 16:39:34 | 593.1s | 58 | 是 |
| a85 | global-ytb | 16:43:58 | 114.8s | 12 |  |
| a85 | global-ytb | 16:47:37 | 30.6s | 4 |  |
| a85 | global-ytb | 16:49:00 | 35.3s | 4 | 是 |
| a85 | global-1111 | 16:39:42 | 120.7s | 13 |  |
| a85 | global-1111 | 16:42:03 | 44.1s | 5 |  |
| a85 | global-1111 | 16:43:58 | 31.8s | 4 |  |
| a85 | global-1111 | 16:44:51 | 283.8s | 28 | 是 |
| a85 | global-cf | 16:39:43 | 30.4s | 4 |  |
| a85 | global-cf | 16:40:52 | 125.2s | 13 |  |
| a85 | global-cf | 16:44:19 | 32.2s | 4 |  |
| a85 | global-cf | 16:45:12 | 29.4s | 4 |  |
| a85 | global-cf | 16:46:03 | 147.7s | 15 |  |
| a85 | global-cf | 16:48:49 | 46.1s | 5 | 是 |
| a85 | global-google | 16:39:44 | 80.1s | 9 |  |
| a85 | global-google | 16:41:23 | 96.3s | 10 |  |
| a85 | global-google | 16:43:58 | 43.0s | 5 |  |
| a85 | global-google | 16:45:01 | 274.7s | 27 | 是 |
| a85 | sub-link | 16:40:27 | 497.9s | 9 | 是 |
| a85 | sub-s3 | 16:40:33 | 497.6s | 9 | 是 |
| mac | cn-ali | 16:38:24 | 590.2s | 60 | 是 |

## 一句话结论

- **a85**：L3 成功 119/366（32.5%），p50 -，p95 -
- **mac**：L3 成功 60/120（50.0%），p50 -，p95 -
