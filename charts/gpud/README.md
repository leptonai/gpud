# Installation

For example, you may use the following commands to install:

```bash
helm install gpud charts/gpud \
--create-namespace \
--namespace gpud-run \
--values charts/gpud/values.yaml \
--set dsName=gpud-run \
--set gpud.GPUD_NO_USAGE_STATS=true \
--set gpud.listen_address=0.0.0.0:15132 \
--set gpud.log_level=info \
--set gpud.web_enable=false \
--set gpud.endpoint=mothership-machine-mothership-machine-dev.cloud.lepton.ai \
--set gpud.enable_auto_update=true \
--set gpud.auto_update_exit_code=0 \
--set 'affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].key=lepton.ai/resource-provider' \
--set 'affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].operator=In' \
--set 'affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].values[0]=ABC'

# to upgrade
helm upgrade gpud charts/gpud ...
```

To check the status:

```bash
kubectl -n gpud-run get pods -o wide
```

To uninstall:

```bash
helm --namespace gpud-run uninstall gpud
```
