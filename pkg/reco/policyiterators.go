package reco

import (
	"context"
	"errors"
	"github.com/flipkart-incubator/ottoscalr/api/v1alpha1"
	"github.com/flipkart-incubator/ottoscalr/pkg/policy"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"time"
)

type Policy struct {
	Name                    string `json:"name"`
	RiskIndex               string `json:"riskIndex"`
	MinReplicaPercentageCut int    `json:"minReplicaPercentageCut"`
	TargetUtilization       int    `json:"targetUtilization"`
}

type PolicyIterator interface {
	NextPolicy(wm WorkloadMeta) (*Policy, error)
}

type PolicyIteratorImpl struct {
	store policy.Store
}

type DefaultPolicyIterator PolicyIteratorImpl

func NewDefaultPolicyIterator(k8sClient client.Client) *DefaultPolicyIterator {
	return &DefaultPolicyIterator{store: policy.NewPolicyStore(k8sClient)}
}

func (pi *DefaultPolicyIterator) NextPolicy(wm WorkloadMeta) (*Policy, error) {
	policy, err := pi.store.GetDefaultPolicy()
	if err != nil {
		return nil, err
	}
	return &Policy{
		Name:                    policy.Name,
		RiskIndex:               policy.Spec.RiskIndex,
		MinReplicaPercentageCut: policy.Spec.MinReplicaPercentageCut,
		TargetUtilization:       policy.Spec.TargetUtilization,
	}, nil
}

type AgingPolicyIterator struct {
	store  policy.Store
	client client.Client
	Age    time.Duration
}

func NewAgingPolicyIterator(k8sClient client.Client, age time.Duration) *AgingPolicyIterator {
	return &AgingPolicyIterator{store: policy.NewPolicyStore(k8sClient), client: k8sClient, Age: age}
}

func (pi *AgingPolicyIterator) NextPolicy(wm WorkloadMeta) (*Policy, error) {
	policyreco := &v1alpha1.PolicyRecommendation{}
	pi.client.Get(context.TODO(), types.NamespacedName{
		Namespace: wm.Namespace,
		Name:      wm.Name,
	}, policyreco)

	expired, err := isAgeBeyondExpiry(policyreco, pi.Age)
	if err != nil {
		return nil, err
	}

	// If the current policy reco is not set return the safest policy
	if len(policyreco.Spec.Policy) == 0 {

		safestPolicy, err := pi.store.GetSafestPolicy()
		if err != nil {
			return nil, err
		}
		log.Printf("Not policy has been configured. Returning safest policy %s", safestPolicy.Name)
		return PolicyFromCR(safestPolicy), nil
	}

	if !expired {
		log.Println("Policy hasn't expired yet")
		p, err := pi.store.GetPolicyByName(policyreco.Spec.Policy)
		if err != nil {
			return nil, err
		}
		return PolicyFromCR(p), nil
	}

	nextPolicy, err := pi.store.GetNextPolicyByName(policyreco.Spec.Policy)
	if err != nil {
		return nil, err
	}

	return PolicyFromCR(nextPolicy), nil
}

func PolicyFromCR(policy *v1alpha1.Policy) *Policy {
	return &Policy{
		Name:                    policy.Name,
		RiskIndex:               policy.Spec.RiskIndex,
		MinReplicaPercentageCut: policy.Spec.MinReplicaPercentageCut,
		TargetUtilization:       policy.Spec.TargetUtilization,
	}
}

func isAgeBeyondExpiry(policyreco *v1alpha1.PolicyRecommendation, age time.Duration) (bool, error) {
	if policyreco == nil {
		return false, errors.New("Policy recommendation is nil")
	}
	// if now() is still before last reco transitionedAt + expiry age
	if policyreco.Spec.TransitionedAt.Add(age).After(metav1.Now().Time) {
		return false, nil
	} else {
		return true, nil
	}
}
