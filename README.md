# Statika

[![Docker Pulls](https://img.shields.io/docker/pulls/fractos/statika.svg?style=for-the-badge)](https://hub.docker.com/r/fractos/statika/)

Statika is a process that will update AWS Classic Load Balancer targets for a set of defined ECS tasks.

The use-case is for ECS being used on a cluster with a single node and Classic Load Balancers being used.
In order to allow rolling deploys to take place then a load balancer must be able to target the dynamic port numbers
that Docker assigns to containers with exposed ports. Since a Classic Load Balancer cannot track the dynamic port
assignment then Statika fills that gap, inspecting running tasks on a container host and updating the service's
load balancer listener target port and health check port when needed.

It should be pointed out that this can be achieved purely in AWS using a combination of Application Load Balancers (for
HTTP/HTTPS) and the new Network Load Balancers (for TCP), which are both able to utilise Target Groups that can track
dynamic port numbers and thus allow rolling deploys on single instances.

There is a Docker wrapper for this at [Statika-Docker](https://github.com/fractos/statika-docker)

## Running

The process needs

Some environment variables to set:

| Name | Description | Example |
| - | - | - |
| CONFIGURATTION_URL | S3 URL of the JSON configuration file | s3://mybucket/statika-config.json |
| SERVICES_URL | S3 URL of the JSON services file | s3://mybucket/statika-services.json |
| AWS_REGION | AWS Region name | eu-west-1 |


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

