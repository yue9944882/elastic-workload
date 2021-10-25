package scale

import v1 "open-cluster-management.io/api/work/v1"

type Scaler func(manifest *v1.Manifest, namespace, name string, replicas int)
