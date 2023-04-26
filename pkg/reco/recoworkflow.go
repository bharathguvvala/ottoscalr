package reco

import (
	"context"
	"errors"
	"fmt"
	v1alpha1 "github.com/flipkart-incubator/ottoscalr/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"log"
	"math"
)

type RecommendationWorkflow interface {
	Execute(ctx context.Context, wm WorkloadMeta) (*v1alpha1.HPAConfiguration, *v1alpha1.HPAConfiguration, *Policy, error)
}

type Recommender interface {
	Recommend(wm WorkloadMeta) (*v1alpha1.HPAConfiguration, error)
}

// TODO(bharathguvvala): make metric scraper part of this struct
type RecommendationWorkflowImpl struct {
	Recommender     Recommender
	PolicyIterators []PolicyIterator
}

type WorkloadMeta struct {
	metav1.TypeMeta
	Name      string
	Namespace string
}

type RecoWorkflowBuilder RecommendationWorkflowImpl

func (b *RecoWorkflowBuilder) AddRecommender(r Recommender) *RecoWorkflowBuilder {
	if b.Recommender == nil {
		b.Recommender = r
		return b
	}
	log.Println("Only one recommender must be added. There's already one configured so ignoring this one.")
	return b
}

func (b *RecoWorkflowBuilder) AddPolicyIterator(p PolicyIterator) *RecoWorkflowBuilder {
	b.PolicyIterators = append(b.PolicyIterators, p)
	return b
}

func (b *RecoWorkflowBuilder) Build() RecommendationWorkflow {
	return &RecommendationWorkflowImpl{
		Recommender:     b.Recommender,
		PolicyIterators: b.PolicyIterators,
	}
}

func NewRecommendationWorkflowBuilder() *RecoWorkflowBuilder {
	return &RecoWorkflowBuilder{}
}

type MockRecommender struct {
	Min       int
	Threshold int
	Max       int
}

func (r *MockRecommender) Recommend(wm WorkloadMeta) (*v1alpha1.HPAConfiguration, error) {
	return &v1alpha1.HPAConfiguration{
		Min:               r.Min,
		Max:               r.Max,
		TargetMetricValue: r.Threshold,
	}, nil
}

func (rw *RecommendationWorkflowImpl) Execute(ctx context.Context, wm WorkloadMeta) (*v1alpha1.HPAConfiguration, *v1alpha1.HPAConfiguration, *Policy, error) {
	if rw.Recommender == nil {
		return nil, nil, nil, errors.New("No recommenders configured in the workflow.")
	}
	recoConfig, err := rw.Recommender.Recommend(wm)
	if err != nil {
		log.Printf("Error while generating recommendation")
		return nil, nil, nil, errors.New("Unable to generate recommendation")
	}
	var nextPolicy *Policy
	for i, pi := range rw.PolicyIterators {
		log.Printf("Running policy iterator %d", i)
		p, err := pi.NextPolicy(wm)
		if err != nil {
			log.Println("Error while generating recommendation")
			return nil, nil, nil, errors.New(fmt.Sprintf("Unable to generate next policy from policy iterator. Cause: %s", err))
		}
		log.Printf("Next Policy recommended by PI %d is %s", i, p.Name)
		nextPolicy = pickSafestPolicy(nextPolicy, p)
		log.Printf("Next Policy after applying PI %d is %s", i, nextPolicy.Name)

	}

	nextConfig := generateNextRecoConfig(recoConfig, nextPolicy, wm)
	return nextConfig, recoConfig, nextPolicy, nil
}

func generateNextRecoConfig(config *v1alpha1.HPAConfiguration, policy *Policy, wm WorkloadMeta) *v1alpha1.HPAConfiguration {
	if shouldApplyReco(config, policy) {
		return config
	} else {
		recoConfig, _ := createRecoConfigFromPolicy(policy, config, wm)
		return recoConfig
	}
}

func createRecoConfigFromPolicy(policy *Policy, recoConfig *v1alpha1.HPAConfiguration, wm WorkloadMeta) (*v1alpha1.HPAConfiguration, error) {
	return &v1alpha1.HPAConfiguration{
		Min:               recoConfig.Max - int(math.Ceil(float64(policy.MinReplicaPercentageCut*(recoConfig.Max-recoConfig.Min)/100))),
		Max:               recoConfig.Max,
		TargetMetricValue: policy.TargetUtilization,
	}, nil
}

// Determines whether the recommendation should take precedence over the nextPolicy
func shouldApplyReco(config *v1alpha1.HPAConfiguration, policy *Policy) bool {
	if policy == nil {
		return true
	}
	// Returns true if the reco is safer than the policy
	if policy.MinReplicaPercentageCut == 100 && config.TargetMetricValue < policy.TargetUtilization {
		return true
	} else {
		return false
	}
}

func pickSafestPolicy(p1, p2 *Policy) *Policy {
	// if either or both of the policies are nil
	if p1 == nil && p2 != nil {
		return p2
	} else if p2 == nil && p1 != nil {
		return p1
	} else if p1 == nil && p2 == nil {
		return nil
	}

	if p1.RiskIndex <= p2.RiskIndex {
		return p1
	} else {
		return p2
	}
}
