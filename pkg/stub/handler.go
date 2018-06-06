package stub

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/xunchangguo/kong-operator/pkg/apis/c2cloud/v1alpha1"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	kongcli "github.com/xunchangguo/kong-operator/pkg/apis/admin"
	kongadminv1 "github.com/xunchangguo/kong-operator/pkg/apis/admin/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
)

func NewHandler() sdk.Handler {
	return &Handler{}
}

type Handler struct {
	// Fill me
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *v1alpha1.Kong:
		kong := o
		logrus.Infof("Kong event received for '%s/%s'",
			kong.Namespace,
			kong.Name)

		if event.Deleted {
			logrus.Infof(
				"Kong deleted event received for '%s/%s'",
				kong.Namespace,
				kong.Name)
		}
		//add or update event
		if err := checkKongConflict(kong); err != nil {
			return err
		}

		logrus.Infof("Retrieving pods with label: '%s'", kong.Spec.LabelSelector)
		podList, err := queryPods(
			kong.Namespace,
			labels.SelectorFromSet(kong.Spec.LabelSelector).String())
		if err != nil {
			logrus.Errorf("Error during querying pods : %v", err)
			return err
		}

		logrus.Infof("Pods found: namespace='%s', %s", kong.Namespace, formatSimplePods(podList.Items))

		if len(podList.Items) == 0 {
			return nil
		}
		processPods(podList.Items, kong)

	case *v1.Pod:
		pod := o
		if !event.Deleted && pod.Status.Phase != v1.PodRunning {
			return nil
		}
		kongs, err := queryKong(pod.Namespace)
		if err != nil {
			logrus.Errorf("Error during querying kongs in namespace '%s': %v", pod.Namespace, err)
			return err
		}
		kongcrd, err := findKongForPod(kongs, pod)
		if err != nil {
			return err
		}
		if kongcrd == nil {
			return nil
		}
		if event.Deleted {
			//pod delete, need 2 delete target
			return processDeletePod(pod, kongcrd)
		}
		if isVerified(pod, kongcrd.Name) {
			logrus.Infof("Ignoring pod '%s/%s' as it has already been processed.", pod.Namespace, pod.Name)
		} else {
			err := processPod(pod, kongcrd)
			if err != nil {
				return err
			}
		}

	}
	return nil
}

func findKongForPod(kongList *v1alpha1.KongList, pod *v1.Pod) (*v1alpha1.Kong, error) {
	var matchIdx []int

	for i := 0; i < len(kongList.Items); i++ {
		kong := kongList.Items[i]

		selector := labels.SelectorFromSet(kong.Spec.LabelSelector)
		if selector.Matches(labels.Set(pod.Labels)) {
			matchIdx = append(matchIdx, i)
		}
	}

	if len(matchIdx) > 1 {
		var list []v1alpha1.Kong
		for _, idx := range matchIdx {
			list = append(list, kongList.Items[idx])
		}
		logrus.Errorf("Multiple kongs for pod '%s/%s' found: %s",
			pod.Namespace, pod.Name, formatSimpleKongs(list))

		return nil, fmt.Errorf("multiple kongs for pod '%s/%s' found", pod.Namespace, pod.Namespace)
	}

	if len(matchIdx) == 0 {
		return nil, nil
	}

	return &kongList.Items[matchIdx[0]], nil
}

// processPods loads or update each pod address info upstream and target
func processPods(pods []v1.Pod, kong *v1alpha1.Kong) {
	logrus.Info("Processing running pods...")

	for i := 0; i < len(pods); i++ {
		pod := &pods[i]

		if isVerified(pod, kong.Name) {
			logrus.Infof("Ignoring pod '%s/%s' as it has already been processed.", pod.Namespace, pod.Name)
		} else {
			err := processPod(pod, kong)
			if err != nil {
				logrus.Warnf("Processing pod failed: %v", err)
			}
		}
	}

	logrus.Info("Processing pods finished.")
}

func processDeletePod(pod *v1.Pod, kong *v1alpha1.Kong) error {
	logrus.Infof("Delete pod '%s'", pod.Name)
	upstream := genUpstreamName(pod)
	ip := pod.Status.PodIP
	ports, err := queryPodTargetPorts(pod)
	if err != nil {
		return err
	}
	portslen := len(ports)
	if portslen > 1 {
		for i := 0; i < portslen; i++ {
			err = deleteKongTarget(kong, fmt.Sprintf("%s-%d", upstream, ports[i]), fmt.Sprintf("%s:%d", ip, ports[i]))
			if err != nil {
				return err
			}
		}
	} else if portslen == 1 {
		err = deleteKongTarget(kong, upstream, fmt.Sprintf("%s:%d", ip, ports[0]))
		if err != nil {
			return err
		}
	}

	return nil
}

func deleteKongTarget(kong *v1alpha1.Kong, upstreamName string, target string) error {
	kongClient, err := kongcli.NewRESTClient(&rest.Config{
		Host:     kong.Spec.KongURL,
		Username: kong.Spec.Username,
		Password: kong.Spec.Password,
		Timeout:  0,
	})
	if err != nil {
		logrus.Errorf("Error creating Kong Rest client: %v", err)
		return err
	}
	logrus.Infof("Delete upstream '%s' target '%s'", upstreamName, target)
	err = kongClient.Targets().Delete(upstreamName, target)
	if err != nil {
		logrus.Errorf("Error delete Kong target: %v", err)
		return err
	}
	return nil
}

func processPod(pod *v1.Pod, kong *v1alpha1.Kong) error {
	logrus.Infof("Inspecting pod '%s'", pod.Name)
	upstream := genUpstreamName(pod)
	apiUri := getApiUri(pod)
	ip := pod.Status.PodIP
	ports, err := queryPodTargetPorts(pod)
	if err != nil {
		logrus.Infof("Mark pod '%s' as verify failed", pod.Name)
		podVerifiedFailed(pod, kong.Name)
		return err
	}
	portslen := len(ports)
	if portslen > 1 {
		for i := 0; i < portslen; i++ {
			//TODO operator挂了或重启了，期间有pod，删除了，怎么删除target，pod新增的话，编辑下CRD就可以了
			//TODO 还是要使用到状态 KongStatus记录target, 同时启动的时候要做同步
			err = dealKongTarget(kong, fmt.Sprintf("%s-%d", upstream, ports[i]), fmt.Sprintf("%s:%d", ip, ports[i]), apiUri)
			if err != nil {
				logrus.Infof("Mark pod '%s' as verify failed", pod.Name)
				podVerifiedFailed(pod, kong.Name)
				return err
			}
		}
	} else if portslen == 1 {
		err = dealKongTarget(kong, upstream, fmt.Sprintf("%s:%d", ip, ports[0]), apiUri)
		if err != nil {
			logrus.Infof("Mark pod '%s' as verify failed", pod.Name)
			podVerifiedFailed(pod, kong.Name)
			return err
		}
	}

	logrus.Infof("Mark pod '%s' as verified", pod.Name)

	return podVerified(pod, kong.Name)
}

func dealKongTarget(kong *v1alpha1.Kong, upstreamName string, target string, apiUri string) error {
	kongClient, err := kongcli.NewRESTClient(&rest.Config{
		Host:     kong.Spec.KongURL,
		Username: kong.Spec.Username,
		Password: kong.Spec.Password,
		Timeout:  0,
	})
	if err != nil {
		logrus.Errorf("Error creating Kong Rest client: %v", err)
	}
	b, res := kongClient.Upstreams().Get(upstreamName)
	if res.StatusCode == http.StatusNotFound {
		upstream := kongadminv1.NewUpstream(upstreamName)

		logrus.Infof("creating Kong Upstream with name %v", upstreamName)
		b, res = kongClient.Upstreams().Create(upstream)
		if res.StatusCode != http.StatusCreated {
			logrus.Errorf("Unexpected error creating Kong Upstream: %v", res)
			return res.Error()
		}
	}

	kongTargets, err := kongClient.Targets().List(nil, upstreamName)
	if err != nil {
		return err
	}
	has := false
	for _, kongTarget := range kongTargets.Items {
		if target == kongTarget.Target {
			has = true
			break
		}
	}
	if has == false {
		target := &kongadminv1.Target{
			Target:   target,
			Upstream: b.ID,
		}
		logrus.Infof("creating Kong Target %v for upstream %v", target, b.ID)
		_, res := kongClient.Targets().Create(target, upstreamName)
		if res.StatusCode != http.StatusCreated {
			logrus.Errorf("Unexpected error creating Kong Target: %v", res)
			return res.Error()
		}
	}

	//TODO add kongAnnotationApiUriKey
	if apiUri != "" {
		_, res := kongClient.Apis().Get(upstreamName)
		if res.StatusCode == http.StatusNotFound {
			api := &kongadminv1.Api{
				Name:        upstreamName,
				Hosts:       []string{},
				Uris:        []string{apiUri},
				Methods:     []string{"GET", "POST", "DELETE", "PUT", "PATCH", "OPTIONS"},
				UpstreamUrl: fmt.Sprintf("http://%s", upstreamName),
				StripUri:    true,
			}
			logrus.Infof("creating Kong apis %s for upstream %s", apiUri, b.ID)
			_, res := kongClient.Apis().Create(api)
			if res.StatusCode != http.StatusCreated {
				logrus.Errorf("Unexpected error creating Kong Apis: %v", res)
				return res.Error()
			}
		}
	}
	return nil
}

func queryPodTargetPorts(pod *v1.Pod) ([]int, error) {
	csize := len(pod.Spec.Containers)
	v, ok := pod.Annotations[kongAnnotationTargetPortKey]
	if ok {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, err
		}

		found := false
		for i := 0; i < csize; i++ {
			container := &pod.Spec.Containers[i]
			for j := 0; j < len(container.Ports); i++ {
				if port == int(container.Ports[j].ContainerPort) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			return []int{port}, nil
		} else {
			return nil, fmt.Errorf("Annotation Target Port[%d] not found", port)
		}
	} else {
		var fports []int
		ports := sets.NewInt()
		firstPort := -1
		for i := 0; i < csize; i++ {
			container := &pod.Spec.Containers[i]
			for j := 0; j < len(container.Ports); j++ {
				if i == 0 && j == 0 {
					firstPort = int(container.Ports[j].ContainerPort)
				}
				if !ports.Has(int(container.Ports[j].ContainerPort)) {
					ports.Insert(int(container.Ports[j].ContainerPort))
				}
			}
			if ports.Has(port_8080) {
				fports = append(fports, port_8080)
			} else if ports.Has(port_80) {
				fports = append(fports, port_80)
			} else if ports.Has(port_8443) {
				fports = append(fports, port_8443)
			} else if ports.Has(port_443) {
				fports = append(fports, port_443)
			}
		}
		if len(fports) > 0 {
			return fports, nil
		} else {
			return []int{firstPort}, nil
		}
	}
}

func getApiUri(pod *v1.Pod) string {
	v, ok := pod.Annotations[kongAnnotationApiUriKey]
	if ok {
		return v
	}
	return ""
}

func genUpstreamName(pod *v1.Pod) string {
	v, ok := pod.Annotations[kongAnnotationUpstreamNameKey]
	if ok {
		return v
	}
	//namespace + app-name
	appCode := getAppCode(pod.Name)
	return fmt.Sprintf("%s-%s", pod.Namespace, appCode)
}

func getAppCode(podName string) string {
	fstr := "-"
	pos := strings.LastIndex(podName, fstr)
	if pos > 0 {
		tmp := podName[0:pos]
		pos = strings.LastIndex(tmp, fstr)
		if pos > 0 {
			return tmp[0:pos]
		}
	}
	return podName
}

// podVerified updates the pod annotations to mark it as verified Failed.
func podVerifiedFailed(pod *v1.Pod, name string) error {
	annotations := map[string]string{
		name: kongAnnotationVerifiedFailed,
	}
	return annotatePod(pod, annotations)
}

// podVerified updates the pod annotations to mark it as verified.
func podVerified(pod *v1.Pod, name string) error {
	annotations := map[string]string{
		name: kongAnnotationVerified,
	}

	return annotatePod(pod, annotations)
}

// annotatePod annotates pod with the given annotations
func annotatePod(pod *v1.Pod, annotations map[string]string) error {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	for key, value := range annotations {
		pod.Annotations[key] = value
	}

	err := sdk.Update(pod)
	if err != nil {
		logrus.Errorf("Updating pod '%s' failed: %v", pod.Name, err)
		return err
	}

	return nil
}

// isVerified returns true of the pod was already processed and verified
func isVerified(pod *v1.Pod, name string) bool {
	v, ok := pod.Annotations[name]

	return ok && (v == kongAnnotationVerified || v == kongAnnotationVerifiedFailed)
}

func checkKongConflict(kong *v1alpha1.Kong) error {
	kongList, err := queryKong(kong.Namespace)
	if err != nil {
		return err
	}
	size := len(kongList.Items)
	if size > 0 {
		for i := 0; i < size; i++ {
			otherKong := kongList.Items[i]
			if otherKong.Name != kong.Name {
				if kong.Spec.KongURL == otherKong.Spec.KongURL {
					logrus.Errorf("kong '%s' ('%s') for NameSpace '%s' already defined",
						kong.Name,
						kong.Spec.KongURL,
						kong.Namespace)

					return fmt.Errorf(
						"kong '%s' ('%s') for NameSpace '%s' already defined",
						kong.Name,
						kong.Spec.KongURL,
						kong.Namespace)
				}
			}
		}
	}

	return nil
}

// queryKong returns KongList from given namespace
func queryKong(namespace string) (*v1alpha1.KongList, error) {
	kongList := v1alpha1.KongList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Kong",
			APIVersion: "c2cloud/v1alpha1",
		},
	}

	listOptions := sdk.WithListOptions(&metav1.ListOptions{
		IncludeUninitialized: false,
	})

	if err := sdk.List(namespace, &kongList, listOptions); err != nil {
		logrus.Errorf("Failed to query kong : %v", err)
		return nil, err
	}

	return &kongList, nil
}

// queryPods returns list of pods according to the labelSelector
func queryPods(namespace, labelSelector string) (*v1.PodList, error) {
	podList := v1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}
	listOptions := sdk.WithListOptions(&metav1.ListOptions{
		LabelSelector:        labelSelector,
		IncludeUninitialized: false,
	})

	err := sdk.List(namespace, &podList, listOptions)
	if err != nil {
		logrus.Errorf("Failed to query pods : %v", err)
		return nil, err
	}

	var filteredPods []v1.Pod

	for i := 0; i < len(podList.Items); i++ {
		if podList.Items[i].Status.Phase == v1.PodRunning {
			filteredPods = append(filteredPods, podList.Items[i])
		}
	}

	podList.Items = filteredPods

	return &podList, nil

}

func formatSimplePods(pods []v1.Pod) string {
	var buffer bytes.Buffer
	buffer.WriteString("(")
	for i := 0; i < len(pods); i++ {
		pod := pods[i]

		if i != 0 {
			buffer.WriteString(",")
		}
		buffer.WriteString(pod.Name)
	}
	buffer.WriteString(")")

	return buffer.String()
}

func formatSimpleKongs(kongs []v1alpha1.Kong) string {
	var buffer bytes.Buffer
	buffer.WriteString("(")
	for i := 0; i < len(kongs); i++ {
		kong := kongs[i]

		if i != 0 {
			buffer.WriteString(",")
		}
		buffer.WriteString(kong.Name)
	}
	buffer.WriteString(")")

	return buffer.String()
}
