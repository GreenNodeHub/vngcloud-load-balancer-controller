package controller

import (
	"context"
	"sync"

	"github.com/anngdinh/operator-helper/k8s"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClientManager struct {
	clients          map[types.NamespacedName]client.Client
	mutex            sync.Mutex
	kubeconfigGetter k8s.Getter
}

var instance *ClientManager
var once sync.Once

// GetClientManager returns the singleton instance of ClientManager
func GetClientManager() *ClientManager {
	once.Do(func() {
		instance = &ClientManager{
			clients:          make(map[types.NamespacedName]client.Client),
			kubeconfigGetter: &k8s.KubeconfigGetter{},
		}
	})
	return instance
}

// GetClient retrieves or creates a client for the given cluster
func (cm *ClientManager) GetClient(Scheme *runtime.Scheme, clusterKey types.NamespacedName) (client.Client, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if client, exists := cm.clients[clusterKey]; exists {
		return client, nil
	}

	restConfig, err := cm.GetRestConfig(clusterKey)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get rest config")
	}

	client, err := client.New(restConfig, client.Options{Scheme: Scheme})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create client")
	}

	cm.clients[clusterKey] = client
	return client, nil
}

func (cm *ClientManager) GetRestConfig(clusterKey types.NamespacedName) (*rest.Config, error) {
	kubeconfig, err := cm.kubeconfigGetter.GetClusterKubeconfig(context.Background(), clusterKey)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get kubeconfig")
	}

	return clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
}
