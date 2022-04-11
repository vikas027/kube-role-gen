## MY UPDATES

### GOAL
- Check whether a clusterrole `restrictions` is present
- If present: while creating the big clusterrole, check whether the `resources` in each rule `apiGroups` (of clusterrole `restrictions`) is present.
    * If yes, skip it
    * If not, let it create
    * At the end, join the rules of clusterrole `restrictions` to the big clusterrole, and create a clusterrole `restricted-cluster-role` yaml file (helpful for troubleshooting)
    * Apply the yaml
- If not present:
    * create a big clusterrole `restricted-cluster-role` yaml file (helpful for troubleshooting)
    * apply the yaml


### TESTING
- Create a temp cluster
```
❯ k3d cluster create
```
NOTE: the above command, sets the KUBECONFIG as well.

- Change the config to run in the cluster
Change the boolean `inClusterMode` to `true`

- Run the code in a pod
```
## Build the binary
❯ make build

## Create RBAC and a pod
❯ kubectl apply -f rbac.yaml

## Copy
❯ kubectl cp kube-role-gen-linux test:/tmp/kube-role-gen
```

- Exec into the pod and run below commands
```bash
❯ cd /tmp

## Help
❯ ./kube-role-gen -h
Usage of ./kube-role-gen:
  -inClusterMode
        run in cluster mode (default true)
  -kubeConfig string
        (optional) absolute path to the kubeConfig file (default "/Users/vikas/.kube/config")
  -name string
        Override the name of the ClusterRole resource that is generated (default "restricted-cluster-role")
  -v    Enable verbose logging

## If there are no restrictions. This is default behaviour
❯ ./kube-role-gen | kubectl apply -f -

OR
❯ ./kube-role-gen
❯ kubetl apply -f role_base.yaml

## If you want restrictions
❯ RESTRICTIONS=role_restrictions.yaml ./kube-role-gen | kubectl apply -f -

OR
❯ RESTRICTIONS=role_restrictions.yaml ./kube-role-gen
❯ kubectl apply -f role_merged.yaml
```

### TODO
- break the code into smaller files
- optionally output in json as well
- have arguments and better defaults
- pre-commit hooks
- write some tests
