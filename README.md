# Statika

Statika is a process that will update AWS Classic Load Balancer targets for a set of defined ECS tasks.

The use-case is for ECS being used on a cluster with only one node and Classic Load Balancers being used.

There is a Docker wrapper for this at [Statika-Docker](https://github.com/fractos/statika-docker)

## Running

Some environment variables to set:

```
	CONFIGURATION_URL=<s3-url> \
	SERVICES_URL=<s3-url> \
	AWS_REGION=<aws-region> \
	fractos/statika:latest
```

## Configuration file format

```
{
  "cluster": "<cluster-name>",
  "sleepTimeSeconds": <time-in-seconds>
}
```

## Services file format

```
[
  {
    "serviceName": "<ecs-service-name>",
    "exposedContainerName": "<container-name-to-balance>",
    "loadBalancerName": "<name-of-classic-load-balancer>"
  }
]
```

This is an array of JSON objects.

