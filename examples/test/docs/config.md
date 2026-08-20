# Config

## Arguments

The following arguments are supported:

| Name | Type | Description | Required | Default |
|------|------|-------------|----------|---------|
| app-name | string | Application display name. | Yes | - |
| debug | bool | Enable debug logging. | No | false |
| server | object | HTTP server configuration. | No | - |
| workers | []object | Background worker pools. | No | - |

### server

The following arguments are supported:

| Name | Type | Description | Required | Default |
|------|------|-------------|----------|---------|
| host | string | Address to bind. | No | localhost |
| port | int | Port to listen on. | No | 8080 |
| pool | object |  | No | - |

#### pool

The following arguments are supported:

| Name | Type | Description | Required | Default |
|------|------|-------------|----------|---------|
| min-size | int |  | No | - |
| max-size | int |  | No | - |

### workers

The following arguments are supported:

| Name | Type | Description | Required | Default |
|------|------|-------------|----------|---------|
| name | string | Worker name. | Yes | - |
| concurrency | int | Number of concurrent jobs. | No | 1 |

