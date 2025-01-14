package heartbeat

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"github.com/pkg/errors"
	kotsv1beta1 "github.com/replicatedhq/kotskinds/apis/kots/v1beta1"
	"github.com/replicatedhq/replicated-sdk/pkg/heartbeat/types"
	"github.com/replicatedhq/replicated-sdk/pkg/logger"
	"github.com/replicatedhq/replicated-sdk/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func InjectHeartbeatInfoHeaders(req *http.Request, heartbeatInfo *types.HeartbeatInfo) {
	headers := GetHeartbeatInfoHeaders(heartbeatInfo)

	for key, value := range headers {
		req.Header.Set(key, value)
	}
}

func GetHeartbeatInfoHeaders(heartbeatInfo *types.HeartbeatInfo) map[string]string {
	headers := make(map[string]string)

	if heartbeatInfo == nil {
		return headers
	}

	headers["X-Replicated-K8sVersion"] = heartbeatInfo.K8sVersion
	headers["X-Replicated-AppStatus"] = heartbeatInfo.AppStatus
	headers["X-Replicated-ClusterID"] = heartbeatInfo.ClusterID
	headers["X-Replicated-InstanceID"] = heartbeatInfo.InstanceID

	if heartbeatInfo.ChannelID != "" {
		headers["X-Replicated-DownstreamChannelID"] = heartbeatInfo.ChannelID
	} else if heartbeatInfo.ChannelName != "" {
		headers["X-Replicated-DownstreamChannelName"] = heartbeatInfo.ChannelName
	}

	headers["X-Replicated-DownstreamChannelSequence"] = strconv.FormatInt(heartbeatInfo.ChannelSequence, 10)

	if heartbeatInfo.K8sDistribution != "" {
		headers["X-Replicated-K8sDistribution"] = heartbeatInfo.K8sDistribution
	}

	return headers
}

func canReport(clientset kubernetes.Interface, namespace string, license *kotsv1beta1.License) (bool, error) {
	if util.IsAirgap() {
		return false, nil
	}

	if util.IsDevEnv() && !util.IsDevLicense(license) {
		// don't send reports from our dev env to our production services even if this is a production license
		return false, nil
	}

	deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), util.GetReplicatedDeploymentName(), metav1.GetOptions{})
	if err != nil {
		return false, errors.Wrap(err, "failed to get replicated deployment")
	}

	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), os.Getenv("REPLICATED_POD_NAME"), metav1.GetOptions{})
	if err != nil {
		return false, errors.Wrap(err, "failed to get replicated pod")
	}

	var podRevision int
	for _, owner := range pod.ObjectMeta.OwnerReferences {
		if owner.APIVersion != "apps/v1" || owner.Kind != "ReplicaSet" {
			continue
		}

		replicaSet, err := clientset.AppsV1().ReplicaSets(namespace).Get(context.TODO(), owner.Name, metav1.GetOptions{})
		if err != nil {
			return false, errors.Wrapf(err, "failed to get replicaset %s", owner.Name)
		}

		revision, ok := replicaSet.Annotations["deployment.kubernetes.io/revision"]
		if !ok {
			continue
		}

		parsed, err := strconv.Atoi(revision)
		if err != nil {
			return false, errors.Wrapf(err, "failed to parse revision annotation for replicaset %s", replicaSet.Name)
		}
		podRevision = parsed
	}

	var deploymentRevision int
	if drv, ok := deployment.Annotations["deployment.kubernetes.io/revision"]; ok {
		parsed, err := strconv.Atoi(drv)
		if err != nil {
			return false, errors.Wrap(err, "failed to parse revision annotation for deployment")
		}
		deploymentRevision = parsed
	}

	if podRevision != deploymentRevision {
		// don't report from sdk instances that are not associated with the current deployment revision.
		// this can happen when a rolling update of the replicated deployment is in progress and the pod is terminating.
		logger.Infof("not reporting from sdk instance with deployment reversion (%d) because a newer deployment reversion (%d) was found", podRevision, deploymentRevision)
		return false, nil
	}

	return true, nil
}
