package kube

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/util/retry"

	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// TODO Make this configurable

type KubeClient interface {

	// Returns current k8s context's ClientSet
	GetClientSet() *kubernetes.Clientset

	// Returns k8s version
	GetServerVersion() (string, error)

	// Returns k8s current context name
	GetContext() string

	// Parses the eks cluster name from the current
	// context name
	GetClusterName() string

	// Returns common pdb info. It would be of form:
	// [][]string{{
	// 	"pdb name", "pdb namespace", "pdb expected pods"
	// }}
	GetPDB() ([][]string, error)
	DeletePod(podName, ns string) error

	// Adds label to k8s node resource
	AddLabelToNode(
		nodeName string,
		labelKey, labelVal string,
		ignoreNotFoundErrors bool,
	) error

	// Marks the node as unschedulable
	CordonNode(nodeName string, ignoreNotFoundErrors bool) error

	// Marks the node as schedulable
	UnCordonNode(nodeName string, ignoreNotFoundErrors bool) error

	// Returns nodes with provided label
	GetNodeByLabel(
		label string,
		ignoreNotFoundErrors bool,
	) ([]corev1.Node, error)

	// Checks if provided k8s node is healthy
	IsNodeHealthy(nodeName string, ignoreNotFoundErrors bool) (bool, error)

	// Returns node count by this label
	GetNodeCountByLabel(label string, ignoreNotFoundErrors bool) (int, error)

	// Evicts all pods in separate go routine in the provided
	// node
	DrainNode(
		ctx context.Context,
		nodeName string,
		ignoreDS, force, deleteLocalData, ignoreNotFoundErrors bool,
		eventLogs chan string,
	) []error

	//DrainNode(nodeName string, ignoreDS, force, deleteLocalData bool) error

	// Checks if some pods are in pending state in all of
	// cluster.
	ArePendingPods() ([]string, error)

	// Checks if the node has the provided label and value
	NodeHasLabel(
		nodeName, labelKey, labelVal string,
		ignoreNotFoundErrors bool,
	) (bool, error)

	// Remove labels from the node
	RemoveLabel(nodeName, labelKey string, ignoreNotFoundErrors bool) error

	// Delete k8s node
	DeleteNode(nodeName string, ignoreNotFoundErrors bool) error

	// Returns value of the label attached to the node
	GetLabelValOfNode(
		nodeName, labelKey string,
		ignoreNotFoundErrors bool,
	) (string, error)

	// Returns an array of deployment names which are using public images
	// The return array is of type
	// [][]string{
	//	"deployment name", "deployment namespace"
	//}
	ListPublicImages() ([][]string, error)

	// Returns health status of all nodes in the cluster
	// The return array is of type
	// [][]string{
	//	"Nodes Healthy", "❌"
	//}
	AreNodeHealthy() ([]string, error)
}

type podSpec struct {
	podName string
	podNs   string
}

type kubeClient struct {
	clientSet      *kubernetes.Clientset
	registry       string
	ignoreNotFound bool
	lock           sync.Mutex
	clusterName    string
}

func NewKubeClient(registry string, ignoreNotFound bool, clusterName string) (*kubeClient, error) {
	cs, err := getClientSet()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create kubernetes client set: %w",
			err,
		)
	}
	return &kubeClient{clientSet: cs, lock: sync.Mutex{}, registry: registry, ignoreNotFound: ignoreNotFound, clusterName: clusterName}, nil
}

func getClientSet() (*kubernetes.Clientset, error) {
	h := filepath.Join(homedir.HomeDir(), ".kube", "config")
	cfg, err := clientcmd.BuildConfigFromFlags("", h)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from flags: %w", err)
	}

	return kubernetes.NewForConfig(cfg)
}

func (c *kubeClient) GetClientSet() *kubernetes.Clientset {
	return c.clientSet
}

func (c *kubeClient) GetServerVersion() (string, error) {
	version, err := c.clientSet.ServerVersion()

	if err != nil {
		return "", err
	}

	return version.String(), err
}

func (c *kubeClient) GetContext() string {
	//return ""
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{
			ExplicitPath: filepath.Join(homedir.HomeDir(), ".kube", "config"),
		},
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		},
	).RawConfig()
	if err != nil {
		log.Fatalf(err.Error())
	}
	return config.CurrentContext
}

func (c *kubeClient) GetPDB() ([][]string, error) {
	pdbs, err := c.clientSet.PolicyV1beta1().
		PodDisruptionBudgets(metav1.NamespaceAll).
		List(context.TODO(), metav1.ListOptions{})
	pdbList := make([][]string, 0)
	for _, pdb := range pdbs.Items {
		if pdb.Status.DisruptionsAllowed == 0 {
			pdbList = append(
				pdbList,
				[]string{
					pdb.Name,
					pdb.Namespace,
					strconv.Itoa(int(pdb.Status.ExpectedPods)),
				},
			)
		}
	}
	if err != nil {
		return nil, err
	}
	return pdbList, nil
}

// Need a better way to get cluster name
func (c *kubeClient) GetClusterName() string {
	return c.clusterName
}

func (c *kubeClient) AddLabelToNode(
	nodeName string,
	labelKey, labelVal string,
	ignoreNotFoundErrors bool,
) error {

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {

		node, err := c.clientSet.CoreV1().
			Nodes().
			Get(context.TODO(), nodeName, metav1.GetOptions{})

		if filterError(err, ignoreNotFoundErrors) != nil {
			return err
		}

		if node.ObjectMeta.Labels[labelKey] != labelVal {
			node.ObjectMeta.Labels[labelKey] = labelVal
		}
		_, err = c.clientSet.CoreV1().
			Nodes().
			Update(context.TODO(), node, metav1.UpdateOptions{})

		return filterError(err, ignoreNotFoundErrors)
	})

	return err
}

func (c *kubeClient) CordonNode(
	nodeName string,
	ignoreNotFoundErrors bool,
) error {

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := c.clientSet.CoreV1().
			Nodes().
			Get(context.TODO(), nodeName, metav1.GetOptions{})

		if filterError(err, ignoreNotFoundErrors) != nil {
			return err
		}
		node.Spec.Unschedulable = true
		_, err = c.clientSet.CoreV1().
			Nodes().
			Update(context.TODO(), node, metav1.UpdateOptions{})

		return filterError(err, ignoreNotFoundErrors)
	})

	return err
}

func (c *kubeClient) GetNodeByLabel(
	label string,
	ignoreNotFoundErrors bool,
) ([]corev1.Node, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	list, err := c.clientSet.CoreV1().
		Nodes().
		List(context.TODO(), metav1.ListOptions{
			LabelSelector: label,
		})
	if filterError(err, ignoreNotFoundErrors) != nil {
		return nil, err
	}
	return list.Items, nil
}

func (c *kubeClient) IsNodeHealthy(
	nodeName string,
	ignoreNotFoundErrors bool,
) (bool, error) {
	node, err := c.clientSet.CoreV1().
		Nodes().
		Get(context.TODO(), nodeName, metav1.GetOptions{})

	if filterError(err, ignoreNotFoundErrors) != nil {
		return false, err
	}
	nodeConditions := node.Status.Conditions

	for _, nodeCondition := range nodeConditions {
		if nodeCondition.Type == "Ready" && nodeCondition.Status == "False" {
			return false, nil
		} else if nodeCondition.Type == "Ready" && nodeCondition.Status == "True" {
			return true, nil
		}
	}

	return false, nil
}

func (c *kubeClient) GetNodeCountByLabel(
	label string,
	ignoreNotFoundErrors bool,
) (int, error) {
	byLabel, err := c.GetNodeByLabel(label, ignoreNotFoundErrors)

	if filterError(err, ignoreNotFoundErrors) != nil {
		return 0, err
	}
	return len(byLabel), nil
}

// TODO Error Handling Improvement
func (c *kubeClient) DrainNode(
	ctx context.Context,
	nodeName string,
	ignoreDS, force, deleteLocalData, ignoreNotFoundErrors bool,
	eventLogs chan string,
) []error {
	pods, err := c.clientSet.CoreV1().
		Pods("").
		List(context.Background(), metav1.ListOptions{
			FieldSelector: "spec.nodeName=" + nodeName,
		})

	if filterError(err, ignoreNotFoundErrors) != nil {
		return []error{err}
	}

	podList := make([]podSpec, 0)
	for _, pod := range pods.Items {
		controller := pod.GetOwnerReferences()
		controllerKind := controller[0].Kind

		if ignoreDS && controllerKind == "DaemonSet" {
			continue
		}
		podList = append(podList, podSpec{
			podName: pod.Name,
			podNs:   pod.Namespace,
		})
	}

	errCh := make(chan error, 1)
	for _, po := range podList {
		pod, err := c.clientSet.CoreV1().
			Pods(po.podNs).
			Get(context.Background(), po.podName, metav1.GetOptions{})

		if filterError(err, ignoreNotFoundErrors) != nil {
			return []error{err}
		}
		go c.EvictPod(po, errCh, pod, force, ignoreNotFoundErrors, eventLogs)

	}

	count := 0
	totalPods := len(podList)
	errors := make([]error, 0)
	for count < totalPods {
		// Block till we evict all pods one by one
		err := <-errCh
		count++

		if filterError(err, ignoreNotFoundErrors) != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// TODO remove redundant vars
func (c *kubeClient) EvictPod(
	pod podSpec,
	errCh chan error,
	po *corev1.Pod,
	force, ignoreNotFoundErrors bool,
	eventLogs chan string,
) {

	evictPolicy := &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.podName,
			Namespace: pod.podNs,
		},
		DeleteOptions: &metav1.DeleteOptions{
			GracePeriodSeconds: nil,
		},
	}
	eventLogs <- fmt.Sprintf("Evicting :: pod %s, ns %s ", pod.podName, pod.podNs)
	err := c.clientSet.PolicyV1beta1().
		Evictions(pod.podNs).
		Evict(context.TODO(), evictPolicy)

	if filterError(err, ignoreNotFoundErrors) != nil {
		// Will force delete if force enabled
		eventLogs <- fmt.Sprintf("Unable to gracefully evict pod %s due to %s", pod.podName, err.Error())
		if force {
			eventLogs <- fmt.Sprintf("Force Delete po %s", pod.podName)
			errCh <- c.DeletePod(pod.podName, pod.podNs)
		} else {
			errCh <- err
		}
	} else {
		errCh <- nil
		//eventLogs <- fmt.Sprintf("Waiting for pod deletion :: pod %s, ns %s ", pod.podNs, pod.podNs)
		err = c.WaitForPodToBeDeleted(*po, 30, 5)

		if filterError(err, ignoreNotFoundErrors) != nil {
			errCh <- err
		} else {
			errCh <- nil
		}
	}
}

func (c *kubeClient) WaitForPodToBeDeleted(
	existingPod corev1.Pod,
	interval, timeout int,
) error {
	intervalDuration := time.Duration(interval) * time.Second
	timeoutDuration := time.Duration(timeout) * time.Minute
	podName := existingPod.Name
	podUid := existingPod.ObjectMeta.UID
	err := wait.PollImmediate(
		intervalDuration,
		timeoutDuration,
		func() (bool, error) {
			p, err := c.clientSet.CoreV1().
				Pods(metav1.NamespaceAll).
				Get(context.Background(), podName, metav1.GetOptions{})
			if apierrors.IsNotFound(err) ||
				(p != nil && p.ObjectMeta.UID != podUid) {
				return true, nil
			} else if err != nil {
				return false, err
			}
			return false, err
		},
	)
	return err
}

func (c *kubeClient) ArePendingPods() ([]string, error) {
	list, err := c.clientSet.CoreV1().
		Pods(metav1.NamespaceAll).
		List(context.TODO(), metav1.ListOptions{FieldSelector: "status.phase=Pending"})
	if err != nil {
		return []string{"No Pending Pods", "❌"}, err
	}
	if len(list.Items) != 0 {
		return []string{"No Pending Pods", "❌"}, err
	} else {
		return []string{"No Pending Pods", "✅"}, err
	}

}

func (c *kubeClient) NodeHasLabel(
	nodeName, labelKey, labelVal string,
	ignoreNotFoundErrors bool,
) (bool, error) {

	c.lock.Lock()
	defer c.lock.Unlock()
	node, err := c.clientSet.CoreV1().
		Nodes().
		Get(context.TODO(), nodeName, metav1.GetOptions{})

	// For situations if ec2 instance is deleted

	if filterError(err, ignoreNotFoundErrors) != nil {
		return false, err
	}
	if value, ok := node.ObjectMeta.Labels[labelKey]; ok && value == labelVal {
		return true, err
	} else {
		return false, err
	}
}

func (c *kubeClient) RemoveLabel(
	nodeName, labelKey string,
	ignoreNotFoundErrors bool,
) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := c.clientSet.CoreV1().
			Nodes().
			Get(context.TODO(), nodeName, metav1.GetOptions{})

		if filterError(err, ignoreNotFoundErrors) != nil {
			return err
		}
		delete(node.ObjectMeta.Labels, labelKey)
		_, err = c.clientSet.CoreV1().
			Nodes().
			Update(context.Background(), node, metav1.UpdateOptions{})

		return filterError(err, ignoreNotFoundErrors)
	})

	return err
}

func (c *kubeClient) DeleteNode(
	nodeName string,
	ignoreNotFoundErrors bool,
) error {
	err := c.clientSet.CoreV1().
		Nodes().
		Delete(context.Background(), nodeName, metav1.DeleteOptions{})
	return filterError(err, ignoreNotFoundErrors)
}

func (c *kubeClient) GetLabelValOfNode(
	nodeName, labelKey string,
	ignoreNotFoundErrors bool,
) (string, error) {
	node, err := c.clientSet.CoreV1().
		Nodes().
		Get(context.TODO(), nodeName, metav1.GetOptions{})

	if filterError(err, ignoreNotFoundErrors) != nil {
		return "", err
	}
	return node.ObjectMeta.Labels[labelKey], nil
}

func (c *kubeClient) UnCordonNode(
	nodeName string,
	ignoreNotFoundErrors bool,
) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := c.clientSet.CoreV1().
			Nodes().
			Get(context.TODO(), nodeName, metav1.GetOptions{})

		if filterError(err, ignoreNotFoundErrors) != nil {
			return err
		}
		node.Spec.Unschedulable = false
		_, err = c.clientSet.CoreV1().
			Nodes().
			Update(context.TODO(), node, metav1.UpdateOptions{})

		return filterError(err, ignoreNotFoundErrors)
	})
	return err
}

func (c *kubeClient) ListPublicImages() ([][]string, error) {

	containerImages := make([][]string, 0)
	deployments, err := c.clientSet.AppsV1().
		Deployments(metav1.NamespaceAll).
		List(context.Background(), metav1.ListOptions{})

	if err != nil {
		return [][]string{}, err
	}

	count := 0
	for _, deployment := range deployments.Items {
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if isPublic(container.Image, c.registry) {
				count += 1
				containerImages = append(
					containerImages,
					[]string{deployment.Name, deployment.Namespace},
				)
			}
		}
	}

	containerImages = append(
		containerImages,
		[]string{"Total", strconv.Itoa(count)},
	)

	return containerImages, nil
}

func (c *kubeClient) AreNodeHealthy() ([]string, error) {

	nodes, _ := c.clientSet.CoreV1().
		Nodes().
		List(context.Background(), metav1.ListOptions{
			LabelSelector: "",
			FieldSelector: "",
		})

	for _, node := range nodes.Items {
		healthy, _ := c.IsNodeHealthy(node.Name, c.ignoreNotFound)
		if !healthy {
			return []string{"Nodes Healthy", "❌"}, nil
		}
	}
	return []string{"Nodes Healthy", "✅"}, nil
}

func (c *kubeClient) DeletePod(podName string, ns string) error {

	return c.clientSet.CoreV1().
		Pods(ns).
		Delete(context.TODO(), podName, metav1.DeleteOptions{})
}

func filterError(err error, ignoreNotFoundErrors bool) error {
	if ignoreNotFoundErrors {

		if apierrors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}
	return err
}

func isPublic(repo, privateRepo string) bool {
	return !strings.Contains(repo, privateRepo)
}
