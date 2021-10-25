```yaml
apiVerion: scheduling.open-cluster-management.io/v1
kind: ElasticWorkload
spec:
  # 要部署的workload在spoke集群中的namespace
  spokeNamespace: default
  # 要管理的Workload目标 ，可以支持:
  #  - 以静态嵌入Deployment
  #  - 复制中枢集群中的某个Deployment再分发
  target:
    type: [ Inline | Import ]
    static: ...
    inline: ...
  # 通过OCM Placement API匹配集群
  placementRef:
    name: ...
  # 分发策略，定义期望的副本分发的策略，可以支持：
  #  - 平均分配
  #  - 按比例分配
  distributionStrategy:
    totalReplicas: 10
    type: [ Even | Propotional ]
  # 弹性策略，定义期望副本未满足的情况下的再分配策略，可以支持：
  #  - 无需再调度
  #  - 简单定义上下限控制
  #  - 定义期望下限(assured)基础上，再定义软上限(borrowable)和硬上限(ceiling),
  budgetStrategy:
    type: [ None | LimitRange | Classful ]
    limitRange:
      min: ...
      max: ...
    classful:
      assured: ...
      borrowable: ...
      ceiling: ...
status:
  # 以ManifestWork形式分发给各个托管集群
  manifestWorks: ...

```