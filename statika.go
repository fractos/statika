package main

import (
	"log"
	"net/url"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws"
	"bytes"
	"io"
	"encoding/json"
	"fmt"
	"time"
	"os"
	"regexp"
)

func main() {
	var statika Statika
	statika.configurationURL = os.Getenv("CONFIGURATION_URL")
	statika.serviceDescriptionURL = os.Getenv("SERVICES_URL")
	statika.region = os.Getenv("AWS_REGION")
	statika.Start()
}

func wrappedLog(s string) {
	log.Printf("statika: %s\n", s)
}

func (statika *Statika) Start() {
	wrappedLog("starting...")

	statika.session = session.New(aws.NewConfig().WithRegion(statika.region))

	configuration, err := readConfiguration(statika)
	if err != nil {
		log.Fatal(err)
		return
	}

	if err != nil {
		log.Fatal(err)
	}

	statika.instanceID, err = getInstanceID()
	if err != nil {
		log.Fatal(err)
		return
	}
	wrappedLog(fmt.Sprintf("instanceID is %s", statika.instanceID))

	statika.containerInstanceID, err = getContainerInstanceID(statika, configuration)
	if err != nil {
		log.Fatal(err)
		return
	}
	if statika.containerInstanceID == "" {
		log.Fatal("could not find our container instance ID")
		return
	}

	_, err = lifecycle(statika, configuration)
	if err != nil {
		log.Fatal(err)
		return
	}
}

func lifecycle(statika *Statika, configuration *Configuration) (int, error) {
	wrappedLog("entered lifecycle")

	for true {
		wrappedLog(fmt.Sprintf("sleeping for %d seconds...", configuration.SleepTimeSeconds))

		time.Sleep(time.Duration(configuration.SleepTimeSeconds) * time.Second)

		services, err := parseServices(statika)
		if err != nil {
			wrappedLog("problem while parsing services")
			return -1, err
		}

		// get list of running tasks on this container instance
		wrappedLog("getting list of running tasks on this container instance")
		instanceTasks, err := getContainerInstanceTasks(statika, configuration)
		if err != nil {
			wrappedLog("problem while getting container instance tasks")
			return -1, err
		}

		var instanceTaskDescriptions []*ecs.Task

		if len(instanceTasks) == 0 {
			wrappedLog("no instance tasks to synchronise")
		} else {
			// get descriptions
			wrappedLog("fetching task descriptions")
			instanceTaskDescriptions, err = getContainerInstanceTaskDescriptions(statika, configuration, instanceTasks)
			if err != nil {
				wrappedLog("problem while getting container instance task descriptions")
				return -1, err
			}
		}

		// for each service
		for _, service := range services {
			wrappedLog(fmt.Sprintf("considering service %s", service.ServiceName))

			// get load balancer description
			loadBalancerDescription, err := getLoadBalancerDescription(statika, service.LoadBalancerName)
			if err != nil {
				wrappedLog("problem while getting load balancer description")
				return -1, err
			}

			var instanceRegistered = false
			for _, candidateInstance := range loadBalancerDescription.Instances {
				if *candidateInstance.InstanceId == statika.instanceID {
					instanceRegistered = true
					break
				}
			}

			// get the list of tasks running for this service on this container instance

			serviceTasks, err := getServiceTasks(statika, configuration, service.ServiceName)
			if err != nil {
				wrappedLog("problem while getting service tasks")
				return -1, err
			}

			if len(serviceTasks) == 0 {
				// no tasks for this service running on the container instance
				// deregister us from the load balancer if we are registered
				if instanceRegistered {
					wrappedLog("deregistering instance from service's load balancer")

					err := deregisterInstanceFromLoadBalancer(statika, service.LoadBalancerName)
					if err != nil {
						wrappedLog("problem while deregistering from load balancer")
						return -1, err
					}
				}

				continue

			} else if len(serviceTasks) > 1 {
				wrappedLog(fmt.Sprintf("there is not exactly 1 instance of this service running (actual: %d)", len(serviceTasks)))
				wrappedLog("skipping this service")
				continue
			}

			wrappedLog(fmt.Sprintf("found running task instance (id: %s)", *serviceTasks[0] ))

			taskDescription := getInstanceTaskDescription(*serviceTasks[0], instanceTaskDescriptions)

			if taskDescription == nil {
				wrappedLog("couldn't find task description")
				continue
			}

			// find container within task description
			for _, container := range taskDescription.Containers {
				if *container.Name == service.ExposedContainerName {
					var hostPort int64
					if len(container.NetworkBindings) > 0 {
						hostPort = *container.NetworkBindings[0].HostPort

						wrappedLog(fmt.Sprintf("found host port value of %d", hostPort))
					} else {
						wrappedLog("couldn't find any network binding on the task description")
						continue
					}

					if !instanceRegistered {
						// register the instance
						wrappedLog("registering instance with service load balancer")

						err := registerInstanceWithLoadBalancer(statika, *loadBalancerDescription.LoadBalancerName)
						if err != nil {
							wrappedLog("problem while registering instance with load balancer")
							return -1, err
						}
					}

					for _, listener := range loadBalancerDescription.ListenerDescriptions {
						if *listener.Listener.InstancePort != hostPort {
							wrappedLog("deleting load balancer listener")

							err := deleteLoadBalancerListener(statika, *loadBalancerDescription.LoadBalancerName, listener)
							if err != nil {
								wrappedLog("problem while deleting load balancer listener")
								return -1, err
							}
							wrappedLog("creating load balancer listener")

							err = createLoadBalancerListener(statika, *loadBalancerDescription.LoadBalancerName, listener, hostPort)
							if err != nil {
								wrappedLog("problem while creating load balancer listener")
								return -1, err
							}

							err = updateLoadBalancerHealthCheck(statika, loadBalancerDescription, hostPort)
							if err != nil {
								wrappedLog("problem while updating load balancer health check")
								return -1, err
							}
						}
					}

				}
			}
		}
	}

	return 0, nil
}

func parseServices(statika *Statika) ([]*ServiceDescription, error) {
	data, err := readFileFromS3(statika.session, statika.serviceDescriptionURL)
	if err != nil {
		wrappedLog("problem while reading file from S3")
		return nil, err
	}
	var serviceDescriptions []*ServiceDescription
	err = json.Unmarshal(data, &serviceDescriptions)
	if err != nil {
		wrappedLog("problem while unmarshalling JSON")
		return nil, err
	}
	return serviceDescriptions, nil
}

func readFileFromS3(sess *session.Session, s3Url string) ([]byte, error) {
	service := s3.New(sess)

	u,_ := url.Parse(s3Url)

	wrappedLog(fmt.Sprintf("reading file from bucket %s key %s", u.Host, u.Path))

	result, err := service.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(u.Host),
		Key:    aws.String(u.Path),
	})
	if err != nil {
		wrappedLog("problem while getting S3 object")
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, result.Body); err != nil {
		wrappedLog("problem while copying S3 object body to buffer")
		return nil, err
	}
	return buf.Bytes(), nil
}

func readConfiguration(statika *Statika) (*Configuration, error) {
	data, err := readFileFromS3(statika.session, statika.configurationURL)
	if err != nil {
		wrappedLog("problem during read from S3")
		return nil, err
	}

	wrappedLog(fmt.Sprintf("config was %s", data))

	var config *Configuration
	if err = json.Unmarshal(data, &config); err != nil {
		wrappedLog("problem during parse of configuration")
		return nil, err
	}

	return config, nil
}

func getInstanceID() (string, error) {
	metadata := ec2metadata.New(session.New())

	id, err := metadata.GetMetadata("instance-id")
	if err != nil {
		wrappedLog("problem while getting EC2 metadata")
		return "", err
	}
	return id, nil
}

func getContainerInstanceID(statika *Statika, configuration *Configuration) (string, error) {
	// because AWS make life difficult - can't get to ecs-agent local port from another container,
	// so get list of container instances and find our instance ID to get the container instance ARN.

	client := ecs.New(statika.session)

	listContainerInstancesInput := ecs.ListContainerInstancesInput{
		Cluster: aws.String(configuration.Cluster),
	}

	containerInstances, err := client.ListContainerInstances(&listContainerInstancesInput)
	if err != nil {
		wrappedLog("problem while listing container instances")
		return "", err
	}

	describeContainerInstanceInput := ecs.DescribeContainerInstancesInput {
		Cluster: aws.String(configuration.Cluster),
		ContainerInstances: containerInstances.ContainerInstanceArns,
	}

	containerInstanceDescriptions, err := client.DescribeContainerInstances(&describeContainerInstanceInput)
	if err != nil {
		wrappedLog("problem while describing container instance")
		return "", err
	}

	for _, containerInstanceDescription := range containerInstanceDescriptions.ContainerInstances {
		if *containerInstanceDescription.Ec2InstanceId == statika.instanceID {
			return *containerInstanceDescription.ContainerInstanceArn, nil
		}
	}

	wrappedLog("couldn't find container instance")
	return "", nil
}

func getContainerInstanceTasks(statika *Statika, configuration *Configuration) ([]*string, error) {
	client := ecs.New(statika.session)

	listTasksInput := ecs.ListTasksInput{
		Cluster: aws.String(configuration.Cluster),
		ContainerInstance: aws.String(statika.containerInstanceID),
	}

	results, err := client.ListTasks(&listTasksInput)
	if err != nil {
		return nil, err
	}

	return results.TaskArns, nil
}

func getContainerInstanceTaskDescriptions(statika *Statika, configuration *Configuration, taskARNs []*string) ([]*ecs.Task, error) {
	client := ecs.New(statika.session)

	describeTasksInput := ecs.DescribeTasksInput{
		Cluster: aws.String(configuration.Cluster),
		Tasks: taskARNs,
	}

	results, err := client.DescribeTasks(&describeTasksInput)
	if err != nil {
		return nil, err
	}

	return results.Tasks, nil
}

func getServiceTasks(statika *Statika, configuration *Configuration, service string) ([]*string, error) {
	client := ecs.New(statika.session)

	listTasksInput := ecs.ListTasksInput{
		Cluster: aws.String(configuration.Cluster),
		ContainerInstance: aws.String(statika.containerInstanceID),
		Family: aws.String(service),
	}

	results, err := client.ListTasks(&listTasksInput)
	if err != nil {
		return nil, err
	}

	return results.TaskArns, nil
}

func getLoadBalancerDescription(statika *Statika, loadBalancerName string) (*elb.LoadBalancerDescription, error) {
	client := elb.New(statika.session)

	describeLoadBalancersInput := elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string { aws.String(loadBalancerName) },
	}

	results, err := client.DescribeLoadBalancers(&describeLoadBalancersInput)
	if err != nil {
		return nil, err
	}

	return results.LoadBalancerDescriptions[0], nil
}

func registerInstanceWithLoadBalancer(statika *Statika, loadBalancerName string) error {
	client := elb.New(statika.session)
	registerInstancesWithLoadBalancerInput := elb.RegisterInstancesWithLoadBalancerInput{
		LoadBalancerName: aws.String(loadBalancerName),
		Instances: []*elb.Instance { { InstanceId: aws.String(statika.instanceID) } },
	}
	_, err := client.RegisterInstancesWithLoadBalancer(&registerInstancesWithLoadBalancerInput)
	return err
}

func deregisterInstanceFromLoadBalancer(statika *Statika, loadBalancerName string) error {
	client := elb.New(statika.session)
	deregisterInstancesFromLoadBalancerInput := elb.DeregisterInstancesFromLoadBalancerInput{
		LoadBalancerName: aws.String(loadBalancerName),
		Instances: []*elb.Instance { { InstanceId: aws.String(statika.instanceID) } },
	}
	_, err := client.DeregisterInstancesFromLoadBalancer(&deregisterInstancesFromLoadBalancerInput)
	return err
}

func deleteLoadBalancerListener(statika *Statika, loadBalancerName string, loadBalancerListenerDescription *elb.ListenerDescription) error {
	client := elb.New(statika.session)
	deleteLoadBalancerListenerInput := elb.DeleteLoadBalancerListenersInput{
		LoadBalancerName: aws.String(loadBalancerName),
		LoadBalancerPorts: []*int64 { loadBalancerListenerDescription.Listener.LoadBalancerPort },
	}

	_, err := client.DeleteLoadBalancerListeners(&deleteLoadBalancerListenerInput)
	return err
}

func createLoadBalancerListener(statika *Statika, loadBalancerName string, loadBalancerListener *elb.ListenerDescription, hostPort int64) error {
	client := elb.New(statika.session)
	createLoadBalancerListenerInput := elb.CreateLoadBalancerListenersInput{
		LoadBalancerName: aws.String(loadBalancerName),
		Listeners: []*elb.Listener {
			{
				LoadBalancerPort: loadBalancerListener.Listener.LoadBalancerPort,
				InstancePort: aws.Int64(hostPort),
				InstanceProtocol: loadBalancerListener.Listener.InstanceProtocol,
				Protocol: loadBalancerListener.Listener.Protocol,
				SSLCertificateId: loadBalancerListener.Listener.SSLCertificateId,
			}},
	}

	_, err := client.CreateLoadBalancerListeners(&createLoadBalancerListenerInput)
	return err
}

func updateLoadBalancerHealthCheck(statika *Statika, loadBalancerDescription *elb.LoadBalancerDescription, hostPort int64) error {
	client := elb.New(statika.session)

	var regexHealthCheck = regexp.MustCompile(`^(?P<protocol>[^:]+):(?P<port>\d+)(?P<path>/.*?)$`)

	n1 := regexHealthCheck.SubexpNames()
	result := regexHealthCheck.FindAllStringSubmatch(*loadBalancerDescription.HealthCheck.Target, -1)[0]

	md := map[string]string{}
	for i, n := range result {
		md[n1[i]] = n
	}

	var targetProtocol = md["protocol"]
	var targetPort = md["port"]
	var targetPath = md["path"]

	wrappedLog(fmt.Sprintf("current health check target is: %s:%s%s", targetProtocol, targetPort, targetPath))

	wrappedLog(fmt.Sprintf("setting health check target to: %s:%d%s", targetProtocol, hostPort, targetPath))

	configureHealthCheckInput := elb.ConfigureHealthCheckInput{
		LoadBalancerName: aws.String(*loadBalancerDescription.LoadBalancerName),
		HealthCheck: &elb.HealthCheck {
			HealthyThreshold: loadBalancerDescription.HealthCheck.HealthyThreshold,
			UnhealthyThreshold: loadBalancerDescription.HealthCheck.UnhealthyThreshold,
			Interval: loadBalancerDescription.HealthCheck.Interval,
			Timeout: loadBalancerDescription.HealthCheck.Timeout,
			Target: aws.String(fmt.Sprintf("%s:%d%s", targetProtocol, hostPort, targetPath)),
		}}

	_, err := client.ConfigureHealthCheck(&configureHealthCheckInput)
	return err
}

func getInstanceTaskDescription(taskARN string, taskDescriptions []*ecs.Task) *ecs.Task {
	for _, task := range taskDescriptions {
		if *task.TaskArn == taskARN {
			return task
		}
	}

	return nil
}
