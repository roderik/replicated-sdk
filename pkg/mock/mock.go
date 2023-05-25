package mock

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	replicatedSecretName  = "replicated"
	replicatedMockDataKey = "REPLICATED_MOCK_DATA"
)

var (
	mock                 *Mock
	replicatedSecretLock = sync.Mutex{}
)

type Mock struct {
	clientset kubernetes.Interface
	namespace string
}

func InitMock(clientset kubernetes.Interface, namespace string) {
	mock = &Mock{
		clientset: clientset,
		namespace: namespace,
	}
}

func MustGetMock() *Mock {
	if mock == nil {
		panic("mock not initialized")
	}
	return mock
}

type MockData struct {
	CurrentRelease    *MockRelease  `json:"currentRelease"`
	DeployedReleases  []MockRelease `json:"deployedReleases"`
	AvailableReleases []MockRelease `json:"availableReleases"`
}

type MockRelease struct {
	VersionLabel         string `json:"versionLabel"`
	ChannelID            string `json:"channelID"`
	ChannelName          string `json:"channelName"`
	ChannelSequence      int    `json:"channelSequence"`
	ReleaseSequence      int    `json:"releaseSequence"`
	IsRequired           bool   `json:"isRequired"`
	ReleaseNotes         string `json:"releaseNotes"`
	HelmReleaseName      string `json:"helmReleaseName,omitempty"`
	HelmReleaseRevision  int    `json:"helmReleaseRevision,omitempty"`
	HelmReleaseNamespace string `json:"helmReleaseNamespace,omitempty"`
}

func (m *Mock) HasMocks() (bool, error) {
	mockData, err := m.GetMockData()
	if err != nil {
		return false, errors.Wrap(err, "failed to get mock data")
	} else if mockData == nil {
		return false, nil
	}

	return true, nil
}

func (m *Mock) GetCurrentRelease() (*MockRelease, error) {
	mockData, err := m.GetMockData()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get mock data")
	} else if mockData == nil {
		return nil, nil
	}

	return mockData.CurrentRelease, nil
}

func (m *Mock) GetAvailableReleases() ([]MockRelease, error) {
	mockData, err := m.GetMockData()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get mock data")
	} else if mockData == nil {
		return nil, nil
	}

	return mockData.AvailableReleases, nil
}

func (m *Mock) GetAllReleases() ([]MockRelease, error) {
	mockData, err := m.GetMockData()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get mock data")
	} else if mockData == nil {
		return nil, nil
	}

	releases := []MockRelease{}
	releases = append(releases, mockData.DeployedReleases...)
	releases = append(releases, mockData.AvailableReleases...)

	return releases, nil
}

func (m *Mock) SetMockData(mockData MockData) error {
	replicatedSecretLock.Lock()
	defer replicatedSecretLock.Unlock()

	b, err := json.Marshal(mockData)
	if err != nil {
		return errors.Wrap(err, "failed to marshal mock data")
	}

	secret, err := m.clientset.CoreV1().Secrets(m.namespace).Get(context.TODO(), replicatedSecretName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get secret replicated-dev")
	}

	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}

	secret.Data[replicatedMockDataKey] = b
	_, err = m.clientset.CoreV1().Secrets(m.namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to update secret replicated-dev")
	}

	return nil
}

func (m *Mock) GetMockData() (*MockData, error) {
	replicatedSecretLock.Lock()
	defer replicatedSecretLock.Unlock()

	secret, err := m.clientset.CoreV1().Secrets(m.namespace).Get(context.TODO(), replicatedSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret replicated-dev")
	}

	b := secret.Data[replicatedMockDataKey]
	if len(b) == 0 {
		return nil, nil
	}

	var mockData MockData
	if err := json.Unmarshal(b, &mockData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal mock data")
	}

	return &mockData, nil
}

func (m *Mock) DeleteMockData() error {
	replicatedSecretLock.Lock()
	defer replicatedSecretLock.Unlock()

	secret, err := m.clientset.CoreV1().Secrets(m.namespace).Get(context.TODO(), replicatedSecretName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get secret replicated-dev")
	}

	delete(secret.Data, replicatedMockDataKey)
	_, err = m.clientset.CoreV1().Secrets(m.namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to update secret replicated-dev")
	}
	return nil
}
