package registry

import (
	"context"
	"testing"

	argov1alpha1 "github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	ottoscaleriov1alpha1 "github.com/flipkart-incubator/ottoscalr/api/v1alpha1"
	"github.com/flipkart-incubator/ottoscalr/pkg/testutil"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	cfg                      *rest.Config
	k8sClient                client.Client
	ctx                      context.Context
	cancel                   context.CancelFunc
	deploymentClientRegistry *DeploymentClientRegistry
	deploymentClient         ObjectClient
	rolloutClient            ObjectClient
)

func TestRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deployment Client Registry Suite")
}

var _ = BeforeSuite(func() {
	logger := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(logger)
	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	var err error
	// cfg is defined in this file globally.
	cfg, ctx, cancel = testutil.SetupSingletonEnvironment()
	Expect(cfg).NotTo(BeNil())

	err = ottoscaleriov1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = argov1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:Scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	deploymentClientRegistry = NewDeploymentClientRegistryBuilder().
		WithK8sClient(k8sClient).
		Build()

	deploymentClient = NewDeploymentClient(k8sClient)
	rolloutClient = NewRolloutClient(k8sClient)

	go func() {
		defer GinkgoRecover()
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testutil.TeardownSingletonEnvironment()
	Expect(err).NotTo(HaveOccurred())
})
