package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

var (
	autoRepairAnnotation  string   = "cassandra-autorepair.jmkeyes.ca/autorepair"
	nodetoolRepairCommand []string = []string{"nodetool", "repair", "-pr"}
)

func k8s() (clientSet kubernetes.Interface, config *rest.Config, err error) {
	// When we're deployed in a Kubernetes cluster, automatically connect to the cluster.
	if _, inCluster := os.LookupEnv("KUBERNETES_SERVICE_HOST"); inCluster {
		log.Println("Running within cluster; using in-cluster configuration")

		if config, err = rest.InClusterConfig(); err != nil {
			log.Printf("Failed to get in-cluster config: %+v", err)
			return nil, nil, err
		}
	} else {
		// Not running within a in cluster? Try loading ~/.kube/config instead.
		kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")

		if config, err = clientcmd.BuildConfigFromFlags("", kubeconfig); err != nil {
			log.Printf("Error loading kubernetes configuration: %+v\n", err)
			return nil, nil, err
		}
	}

	// Build a client from the config.
	if clientSet, err = kubernetes.NewForConfig(config); err != nil {
		log.Printf("Failed to construct the clientSet: %+v", err)
		return nil, nil, err
	}

	// Return early.
	return clientSet, config, nil
}

func selectContainer(pod v1.Pod, defaultContainerName string) (string, error) {
	// If there's only one container, use it.
	if len(pod.Spec.Containers) == 1 {
		return pod.Spec.Containers[0].Name, nil
	}

	// If the annotation named a specific container, use it.
	for _, container := range pod.Spec.Containers {
		if container.Name == defaultContainerName {
			return defaultContainerName, nil
		}
	}

	// Otherwise return an error.
	return "", errors.New("no such container")
}

func main() {
	// Create a base context.
	ctx := context.Background()

	// Create a Kubernetes API client.
	clientSet, config, err := k8s()

	if err != nil {
		log.Fatalf("Unable to create Kubernetes API client: %+v\n", err)
	}

	// Detect the namespace we're running in.
	namespace, ok := os.LookupEnv("POD_NAMESPACE")

	if !ok {
		log.Fatalf("Unable to detect namespace: %+v\n", err)
	}

	// List all pods within this namespace.
	pods, err := clientSet.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		log.Fatalf("Unable to list pods in namespace %s: %s\n", namespace, err.Error())
	}

	// For each of the pods listed...
	for _, pod := range pods.Items {
		// Find pods with the annotation we're looking for that are also running/ready phases.
		if value, ok := pod.Annotations[autoRepairAnnotation]; ok && pod.Status.Phase == v1.PodRunning {
			container, err := selectContainer(pod, value)

			if err != nil {
				log.Printf("Unable to match a container in %s: %+v", pod.Name, err)
				continue
			}

			log.Printf("Started repair of %s/%s [%s]\n", pod.Namespace, pod.Name, container)

			// Set up the request we need to exec into the pod.
			req := clientSet.CoreV1().RESTClient().Post().Resource("pods").
				Name(pod.Name).
				Namespace(namespace).
				SubResource("exec").
				VersionedParams(&v1.PodExecOptions{
					Command:   nodetoolRepairCommand,
					Container: container,
					Stdin:     false,
					Stdout:    true,
					Stderr:    true,
					TTY:       true,
				}, scheme.ParameterCodec)

			// Kick off the repair command.
			exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())

			if err != nil {
				log.Printf("Failed to exec: %+v", err)
				return
			}

			// Create a pipe for reading the command output.
			reader, writer := io.Pipe()
			defer func() {
				if err := reader.Close(); err != nil {
					log.Fatalf("Unable to close pipe reader: %+v", err)
				}
			}()

			// Consume the command output.
			go func() {
				defer func() {
					if err := writer.Close(); err != nil {
						log.Fatalf("Unable to close pipe writer: %+v", err)
					}
				}()

				streamOptions := remotecommand.StreamOptions{
					Stdin:  nil,
					Stdout: writer,
					Stderr: writer,
				}

				if err := exec.StreamWithContext(ctx, streamOptions); err != nil {
					log.Printf("Failed to get result: %+v", err)
				}
			}()

			// Scan the output from the executed command.
			scanner := bufio.NewScanner(reader)

			for scanner.Scan() {
				log.Printf("   %s\n", scanner.Text())
			}

			log.Printf("Finished repair of %s/%s [%s]\n", pod.Namespace, pod.Name, container)
		}
	}
}
