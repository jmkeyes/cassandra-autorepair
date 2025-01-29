Cassandra Auto-Repair
=====================

This executes `nodetool repair -pr` inside every annotated Cassandra container.

When run, this application will:

  1. Connect to the Kubernetes API within the cluster.
  2. List the pods annotated with `cassandra-autorepair.jmkeyes.ca/autorepair`.
  3. Run `nodetool repair -pr` on each pod sequentially.

This avoids having to expose a raw JMX connector from each Cassandra container.

To apply it to the Pod template of a StatefulSet named `cassandra`:

```json
$ kubectl patch sts/cassandra --patch-file=/dev/stdin <<EOF
{
  "spec": {
    "template": {
      "metadata": {
        "annotations": {
          "cassandra-autorepair.jmkeyes.ca/autorepair": "true"
        }
      }
    }
  }
}
```

Usage Instructions
------------------

```yaml
$ kubectl apply -f <<EOF
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: cassandra-autorepair
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: cassandra-autorepair-role
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]
- apiGroups: [""]
  resources: ["pods/exec"]
  verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: cassandra-autorepair-role-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: cassandra-autorepair-role
subjects:
- kind: ServiceAccount
  name: cassandra-autorepair
---
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  name: cassandra-autorepair
spec:
  # Run at midnight UTC every day.
  schedule: "00 0 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: cassandra-autorepair
          restartPolicy: OnFailure
          containers:
          - name: cassandra-autorepair
            image: cassandra-autorepair:1.0.0
            env:
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
EOF
```
