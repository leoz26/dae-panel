# S1 中期对比：.85(dae) vs Mac 经 .11(OpenClash)

| 探针 | 主机 | 轮数 | 层 |
|---|---|---|---|
| a85s1 | Debian-te | - | l0,l1,l2,l3,l4 |
| via11 | sthhodeMac-mini.local | - | l1,l2,l3,l4 |

## 分层聚合（失败率 / p50 / p95 / max）

| 探针 | 层 | 目标 | 成功 | 失败 | 失败率 | p50 | p95 | max | 主要错误 |
|---|---|---|---|---|---|---|---|---|---|
| a85s1 | l0 | cn-ali | 19 | 0 | 0.0% | 7.7ms | 9.2ms | 9.4ms |  |
| a85s1 | l0 | cn-baidu | 19 | 0 | 0.0% | 7.8ms | 8.7ms | 8.9ms |  |
| a85s1 | l0 | global-1111 | 19 | 0 | 0.0% | 217.0ms | 233.0ms | 235.0ms |  |
| a85s1 | l0 | global-cf | 18 | 1 | 5.3% | 153.0ms | 157.0ms | 159.0ms |  |
| a85s1 | l0 | global-google | 0 | 19 | 100.0% | - | - | - |  |
| a85s1 | l0 | global-ytb | 0 | 19 | 100.0% | - | - | - |  |
| a85s1 | l0 | sub-link | 3 | 0 | 0.0% | 164.0ms | 164.0ms | 164.0ms |  |
| a85s1 | l0 | sub-s3 | 3 | 0 | 0.0% | 195.0ms | 215.0ms | 215.0ms |  |
| a85s1 | l1 | cn-ali | 19 | 0 | 0.0% | 8.2ms | 8.8ms | 9.1ms |  |
| a85s1 | l1 | cn-baidu | 19 | 0 | 0.0% | 9.3ms | 11.7ms | 11.9ms |  |
| a85s1 | l1 | global-1111 | 19 | 0 | 0.0% | 0.3ms | 1.2ms | 1.2ms |  |
| a85s1 | l1 | global-cf | 19 | 0 | 0.0% | 1.4ms | 2.9ms | 3.3ms |  |
| a85s1 | l1 | global-google | 19 | 0 | 0.0% | 1.1ms | 1.4ms | 12.3ms |  |
| a85s1 | l1 | global-ytb | 19 | 0 | 0.0% | 1.1ms | 1.4ms | 1.5ms |  |
| a85s1 | l1 | sub-link | 3 | 0 | 0.0% | 1.2ms | 1.3ms | 1.3ms |  |
| a85s1 | l1 | sub-s3 | 3 | 0 | 0.0% | 1.6ms | 1.6ms | 1.6ms |  |
| a85s1 | l2 | cn-ali | 19 | 0 | 0.0% | 22.6ms | 27.2ms | 30.6ms |  |
| a85s1 | l2 | cn-baidu | 19 | 0 | 0.0% | 32.1ms | 37.8ms | 38.7ms |  |
| a85s1 | l2 | global-1111 | 19 | 0 | 0.0% | 561.0ms | 1847.8ms | 1896.8ms |  |
| a85s1 | l2 | global-cf | 19 | 0 | 0.0% | 568.1ms | 1834.7ms | 2704.8ms |  |
| a85s1 | l2 | global-google | 19 | 0 | 0.0% | 555.0ms | 1257.2ms | 1402.2ms |  |
| a85s1 | l2 | global-ytb | 19 | 0 | 0.0% | 795.2ms | 1987.8ms | 2806.8ms |  |
| a85s1 | l2 | sub-link | 3 | 0 | 0.0% | 783.7ms | 2362.0ms | 2362.0ms |  |
| a85s1 | l2 | sub-s3 | 3 | 0 | 0.0% | 541.8ms | 862.7ms | 862.7ms |  |
| a85s1 | l3 | cn-baidu | 19 | 0 | 0.0% | - | - | - |  |
| a85s1 | l3 | global-1111 | 7 | 12 | 63.2% | - | - | - | EOFNoResponse |
| a85s1 | l3 | global-cf | 6 | 13 | 68.4% | - | - | - | EOFNoResponse |
| a85s1 | l3 | global-google | 5 | 14 | 73.7% | - | - | - | EOFNoResponse |
| a85s1 | l3 | global-ytb | 5 | 14 | 73.7% | - | - | - | TimeoutError |
| a85s1 | l3 | sub-link | 0 | 3 | 100.0% | - | - | - |  |
| a85s1 | l3 | sub-s3 | 0 | 3 | 100.0% | - | - | - | EOFNoResponse |
| a85s1 | l4 | dns:www.baidu.com | 38 | 0 | 0.0% | 1.2ms | 2.6ms | 4.1ms |  |
| a85s1 | l4 | dns:www.cloudflare.com | 38 | 0 | 0.0% | 0.7ms | 0.9ms | 1.6ms |  |
| a85s1 | l4 | dns:www.youtube.com | 38 | 0 | 0.0% | 0.6ms | 1.1ms | 2.7ms |  |
| via11 | l1 | cn-ali | 53 | 0 | 0.0% | 11.7ms | 12.7ms | 13.6ms |  |
| via11 | l1 | cn-baidu | 53 | 0 | 0.0% | 16.9ms | 25.4ms | 39.7ms |  |
| via11 | l1 | global-1111 | 53 | 0 | 0.0% | 3.9ms | 7.4ms | 7.8ms |  |
| via11 | l1 | global-cf | 53 | 0 | 0.0% | 8.6ms | 14.2ms | 24.5ms |  |
| via11 | l1 | global-google | 53 | 0 | 0.0% | 8.6ms | 13.2ms | 20.7ms |  |
| via11 | l1 | global-ytb | 53 | 0 | 0.0% | 7.7ms | 10.2ms | 14.1ms |  |
| via11 | l1 | sub-link | 8 | 0 | 0.0% | 10.0ms | 20.3ms | 20.3ms |  |
| via11 | l1 | sub-s3 | 8 | 0 | 0.0% | 9.1ms | 15.4ms | 15.4ms |  |
| via11 | l2 | cn-ali | 53 | 0 | 0.0% | 41.7ms | 46.3ms | 47.6ms |  |
| via11 | l2 | cn-baidu | 53 | 0 | 0.0% | 48.4ms | 57.9ms | 119.8ms |  |
| via11 | l2 | global-1111 | 53 | 0 | 0.0% | 509.2ms | 827.9ms | 1531.8ms |  |
| via11 | l2 | global-cf | 51 | 2 | 3.8% | 503.8ms | 781.5ms | 896.4ms | timeout |
| via11 | l2 | global-google | 53 | 0 | 0.0% | 530.5ms | 835.6ms | 1169.3ms |  |
| via11 | l2 | global-ytb | 53 | 0 | 0.0% | 537.1ms | 873.2ms | 1415.8ms |  |
| via11 | l2 | sub-link | 8 | 0 | 0.0% | 945.1ms | 993.3ms | 993.3ms |  |
| via11 | l2 | sub-s3 | 8 | 0 | 0.0% | 537.1ms | 829.6ms | 829.6ms |  |
| via11 | l3 | cn-baidu | 53 | 0 | 0.0% | - | - | - |  |
| via11 | l3 | global-1111 | 53 | 0 | 0.0% | - | - | - |  |
| via11 | l3 | global-cf | 53 | 0 | 0.0% | - | - | - |  |
| via11 | l3 | global-google | 53 | 0 | 0.0% | - | - | - |  |
| via11 | l3 | global-ytb | 52 | 1 | 1.9% | - | - | - | timeout |
| via11 | l3 | sub-link | 0 | 8 | 100.0% | - | - | - |  |
| via11 | l3 | sub-s3 | 0 | 8 | 100.0% | - | - | - |  |
| via11 | l4 | dns:www.baidu.com | 106 | 0 | 0.0% | 4.1ms | 5.8ms | 12.8ms |  |
| via11 | l4 | dns:www.cloudflare.com | 106 | 0 | 0.0% | 3.9ms | 5.0ms | 9.8ms |  |
| via11 | l4 | dns:www.youtube.com | 106 | 0 | 0.0% | 3.7ms | 4.8ms | 10.5ms |  |

## 断流事件（同一目标连续 ≥3 次失败，基于 l3）

| 探针 | 目标 | 开始(UTC) | 持续 | 失败次数 | 未恢复 |
|---|---|---|---|---|---|
| a85s1 | global-1111 | 17:21:14 | 109.1s | 4 |  |
| a85s1 | global-1111 | 17:24:13 | 96.4s | 4 | 是 |
| a85s1 | global-google | 17:19:10 | 165.0s | 6 |  |
| a85s1 | global-google | 17:23:02 | 166.8s | 6 | 是 |
| a85s1 | global-cf | 17:20:46 | 305.9s | 10 | 是 |
| a85s1 | global-ytb | 17:17:18 | 65.3s | 3 |  |
| a85s1 | global-ytb | 17:19:22 | 309.9s | 10 |  |
| a85s1 | sub-s3 | 17:18:50 | 397.3s | 3 | 是 |
| a85s1 | sub-link | 17:18:53 | 381.3s | 3 | 是 |
| via11 | sub-s3 | 17:17:46 | 433.2s | 8 | 是 |
| via11 | sub-link | 17:17:47 | 433.8s | 8 | 是 |

## 一句话结论

- **a85s1**：L3 成功 42/101（41.6%），p50 -，p95 -
- **via11**：L3 成功 264/281（94.0%），p50 -，p95 -
