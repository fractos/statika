# Statika

[![Docker Pulls](https://img.shields.io/docker/pulls/fractos/statika.svg?style=for-the-badge)](https://hub.docker.com/r/fractos/statika/)

Statika is a process that will update AWS Classic Load Balancer targets for a set of defined ECS tasks.

The use-case is for ECS being used on single-node clusters and Classic Load Balancers being used.

In order to allow rolling deploys to take place then a load balancer must be able to target the dynamic port numbers
that Docker assigns to containers with exposed ports. Since a Classic Load Balancer cannot track the dynamic port
assignment then Statika fills that gap, inspecting running tasks on a container host and updating the service's
load balancer listener target port and health check port when needed.

Statika should be launched on each host in a cluster so it can manage the process of registering and deregistering the
host with the service's load balancer, and setting the ports on the load balancer when the running tasks change.

It should be pointed out that this can be achieved purely in AWS using a combination of Application Load Balancers (for
HTTP/HTTPS) and the new Network Load Balancers (for TCP), which are both able to utilise Target Groups that can track
dynamic port numbers and thus allow rolling deploys on single instances.

UPDATE: Except that NLBs aren't fit for this purpose - they can't be used to target a service on the same host as the source of the request - i.e. they can't do loopback. This is really quite bad for service discovery.

There is a Docker wrapper for this at [Statika-Docker](https://github.com/fractos/statika-docker)

## Running

Statika needs the following permissions:

| Action | Resource |
| - | - |
| elb.ConfigureHealthCheck | (load balancer ARN) |
| elb.CreateLoadBalancerListener | (load balancer ARN) |
| elb.DeleteLoadBalancerListeners | (load balancer ARN) |
| elb.DeregisterInstancesFromLoadBalancer | (load balancer ARN) |
| elb.RegisterInstancesWithLoadBalancer | (load balancer ARN) |
| elb.DescribeLoadBalancers | (load balancer ARN) |
| ecs.ListTasks | (cluster ARN) |
| ecs.DescribeTasks | (cluster ARN) |
| ecs.DescribeContainerInstances | (cluster ARN) |
| ecs.ListContainerInstances | (cluster ARN) |
| s3.GetObject | (bucket ARN) |

Some environment variables to set:

| Name | Description | Example |
| - | - | - |
| CONFIGURATION_URL | S3 URL of the JSON configuration file | s3://mybucket/statika-config.json |
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
    "family": "<ecs-task-family-name>",
    "exposedContainerName": "<container-name-to-balance>",
    "loadBalancerName": "<name-of-classic-load-balancer>"
  }
]
```

This is an array of JSON objects.

