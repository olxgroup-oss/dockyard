# Frequently Asked Questions


### What happens if dockyard is interrupted while a rollout was in progress ?
Dockyard maintains the state of rollout by labelling the node with a label `dockyard.io/node-state`, using this label it decides if node is already updated or not.
So even if there is some interruption during the rollout, dockyard will ignore the updated nodes and will only update the older nodes. 

### Will there be any configuration drift while executing asg rollouts ?
During the rollout, dockyard modifies the state of ASG in Prerollout and rollout phase. All these changes are temporary and is reverted back in post rollout phase.
So as to ensure that state remains unchanged once the entire rollout is completed.

### Can we do multiple asg rollouts in parallel ?
With current iteration, parallel asg rollouts are not supported.

### Can we ignore pdb during rollouts ?
By default, all PDBs are honored but we can override this behaviour by setting ASG_ROLLOUT.FORCE_DELETE_PODS to true. If this variable is truthy then dockyard will first try to evict the workload gracefully using eviction api and if it fails to evict it gracefully then it'll call pod deletion api.

### How are pods terminated during rollouts ?
Pods are gracefully terminated using the eviction api respecting terminationGracePeriodSeconds used by workload.

### Dockyard takes too much time during the rollout  ?
Currently, dockyard rolls out new nodes in a batch of size 1 i.e dockyard will wait for 1 node to get fully updated before starting a new rollout. ( At max 1 extra node is required to perform the entire asg rollout ).
Usually it takes ~5 min to provision a new ec2 instance and get it registered with k8s cluster. So untill we have a new healthy node in the cluster dockyard would simply wait and won't start the workload eviction.  
We are planning to support dynamic batch sizes in future iterations of dockyard.





