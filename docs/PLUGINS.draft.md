
## Examples: test plugins locally

To validate sample/example configuration

```bash
gpud cs
```

```text
+-----------------------------------+-----------+----------+---------+----------+---------+
|             COMPONENT             |   TYPE    | RUN MODE | TIMEOUT | INTERVAL |  VALID  |
+-----------------------------------+-----------+----------+---------+----------+---------+
|           test-healthy            | component |   auto   |  1m0s   |  10m0s   | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
|          test-unhealthy           | component |   auto   |  1m0s   |  10m0s   | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
| test-unhealthy-with-missing-field | component |   auto   |  1m0s   |  10m0s   | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
|              exit-0               | component |   auto   |  1m0s   | 1h40m0s  | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
|              exit-1               | component |   auto   |  1m0s   | 1h40m0s  | ✔ valid |
+-----------------------------------+-----------+----------+---------+----------+---------+
```

To run and see the results of the sample configuration:

```bash
gpud cs -r
```

```text
### Component "test-healthy" output

hello world no JSON yet
{"name": "test", "result": "healthy", "passed": true, "action": "reboot me 1", "suggestion": "reboot me 2"}
thank you



### Component "test-unhealthy" output

hello world no JSON yet
{"name": "test", "result": "unhealthy", "passed": false, "action": "reboot me 1", "suggestion": "reboot me 2"}
done



### Component "test-unhealthy-with-missing-field" output

hello world no JSON yet
{"name": "test", "result": "unhealthy", "passed": false}
done



### Component "exit-0" output

{"description": "calling exit 0"}



### Component "exit-1" output

{"description": "calling exit 1"}



### Results

+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|             COMPONENT             | HEALTH STATE |                   SUMMARY                   |     ERROR     | RUN MODE |                                               EXTRA INFO                                                |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|           test-healthy            |  ✔ Healthy   |                     ok                      |               |   auto   |  {"action":"reboot me 1","name":"test","passed":"true","result":"healthy","suggestion":"reboot me 2"}   |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|          test-unhealthy           | ✘ Unhealthy  |          unexpected plugin output           |               |   auto   | {"action":"reboot me 1","name":"test","passed":"false","result":"unhealthy","suggestion":"reboot me 2"} |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
| test-unhealthy-with-missing-field | ✘ Unhealthy  |          unexpected plugin output           |               |   auto   |                   {"name":"test","nothere":"","passed":"false","result":"unhealthy"}                    |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|              exit-0               |  ✔ Healthy   |                     ok                      |               |   auto   |                                    {"description":"calling exit 0"}                                     |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
|              exit-1               | ✘ Unhealthy  | error executing state plugin (exit code: 1) | exit status 1 |   auto   |                                    {"description":"calling exit 1"}                                     |
+-----------------------------------+--------------+---------------------------------------------+---------------+----------+---------------------------------------------------------------------------------------------------------+
```

To validate and run your own configuration:

```bash
gpud cs ./pkg/custom-plugins/testdata/plugins.plaintext.2.regex.yaml
gpud cs ./pkg/custom-plugins/testdata/plugins.plaintext.2.regex.yaml -r
```

To start GPUd with the local plugin configuration file:

```bash
gpud run \
--enable-auto-update=false \
--plugin-specs-file=./pkg/custom-plugins/testdata/plugins.plaintext.2.regex.yaml
```

To trigger the registered plugin manually (works for any component):

```bash
curl -s -kL https://localhost:15132/v1/components/trigger-check?componentName=exit-1 | jq
```

```json
[
  {
    "time": "...",
    "component": "exit-1",
    "component_type": "custom-plugin",
    "name": "exit-1",
    "run_mode": "auto",
    "health": "Unhealthy",
    "reason": "error executing state plugin (exit code: 1)",
    "error": "exit status 1",
    "suggested_actions": {
      "description": "reboot me",
      "repair_actions": [
        "REBOOT_SYSTEM"
      ]
    },
    "extra_info": {
      "action": "reboot me",
      "description": "about to fail with exit code 1"
    }
  }
]
```

Other endpoints:

```bash
curl -kL https://localhost:15132/healthz
curl -kL https://localhost:15132/v1/states | jq | less
curl -kL https://localhost:15132/v1/metrics | jq | less
curl -kL https://localhost:15132/v1/events | jq | less
curl -kL https://localhost:15132/metrics
```
