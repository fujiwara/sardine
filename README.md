# sardine

Mackerel plugin metrics aggregator with CloudWatch.

## Configuration

```toml
# config.toml

[plugin.metrics.memcached]
command = "mackerel-plugin-memcached --host localhost --port 11211"
dimentions = [
  "ClusterName=mycluster",
  "ClusterName=mycluster,AvailabilityZone=az-a"
] # "Name=Value[,Name=Value...]"

[plugin.metrics.xxxx]
command = "...."

[plugin.check.memcached]
namespace = "memcahed/check" # required
dimentions = ["ClusterName=mycluster"]
command = "memping -s localhost:11211"
```

```console
$ sardine-agent -config config.toml
```

AWS credentials for access to CloudWatch are read from environment variables or instance profile.

- `AWS_REGION`: required. e.g. `ap-northeast-1`

## How sardine works

sardine-agent works as below.

1. Execute a `command` for each `[plugin.metrics.*]` sections.
  - interval 60 sec.
1. Put metrics got from command's output to CloudWatch metrics.
  - e.g. `memcached.cmd.cmd_get  10.0  1512057958` put as
    - Namespace: memcached/cmd
    - MetricName: cmd_get
    - Value: 10.0
    - Timestamp: 2017-12-01T16:05:58Z
1. Execute a `command` for each `[plugin.check.*]` sections.
  - interval 60 sec.
1. Put a command's result to CloudWatch metrics.
  - e.g.
    - Namespace: memcached/check
    - MetricName: CheckFailed
    - Value: 1
    - Timestamp: 2017-12-01T16:05:58Z
  - exit status of command maps to CloudWatch metric name as below
    - 0 : CheckOK
    - 1 : CheckFailed
    - 2 : CheckWarning
    - other : CheckUnknown
  - metric Value is always 1

## Author

Fujiwara Shunichiro <fujiwara.shunichiro@gmail.com>

## License

Copyright 2017 Fujiwara Shunichiro

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

nless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
