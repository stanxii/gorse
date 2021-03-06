package core

import (
	"math"
	"sort"
)

const (
	basic    = 0
	centered = 1
	baseline = 2
)

type KNN struct {
	option     Option
	tpe        int
	globalMean float64
	sims       [][]float64
	ratings    [][]float64
	trainSet   TrainSet
	means      []float64 // Centered KNN: user (item) mean
	bias       []float64 // KNN Baseline: bias
}

type CandidateSet struct {
	similarities []float64
	candidates   []int
}

func NewCandidateSet(sim []float64, candidates []int) *CandidateSet {
	neighbors := CandidateSet{}
	neighbors.similarities = sim
	neighbors.candidates = candidates
	return &neighbors
}

func (n *CandidateSet) Len() int {
	return len(n.candidates)
}

func (n *CandidateSet) Less(i, j int) bool {
	return n.similarities[n.candidates[i]] > n.similarities[n.candidates[j]]
}

func (n *CandidateSet) Swap(i, j int) {
	n.candidates[i], n.candidates[j] = n.candidates[j], n.candidates[i]
}

func NewKNN() *KNN {
	knn := new(KNN)
	knn.tpe = basic
	return knn
}

func NewKNNWithMean() *KNN {
	knn := new(KNN)
	knn.tpe = centered
	return knn
}

func NewKNNBaseLine() *KNN {
	knn := new(KNN)
	knn.tpe = baseline
	return knn
}

func (knn *KNN) Predict(userId int, itemId int) float64 {
	innerUserId := knn.trainSet.ConvertUserId(userId)
	innerItemId := knn.trainSet.ConvertItemId(itemId)
	// Set user based or item based
	var leftId, rightId int
	if knn.option.userBased {
		leftId, rightId = innerUserId, innerItemId
	} else {
		leftId, rightId = innerItemId, innerUserId
	}
	if leftId == noBody || rightId == noBody {
		return knn.globalMean
	}
	// Find user (item) interacted with item (user)
	candidates := make([]int, 0)
	for otherId := range knn.ratings {
		if !math.IsNaN(knn.ratings[otherId][rightId]) && !math.IsNaN(knn.sims[leftId][otherId]) {
			candidates = append(candidates, otherId)
		}
	}
	// Set global globalMean for a user (item) with the number of neighborhoods less than min k
	if len(candidates) <= knn.option.minK {
		return knn.globalMean
	}
	// Sort users (items) by similarity
	candidateSet := NewCandidateSet(knn.sims[leftId], candidates)
	sort.Sort(candidateSet)
	// Find neighborhoods
	numNeighbors := knn.option.k
	if numNeighbors > candidateSet.Len() {
		numNeighbors = candidateSet.Len()
	}
	// Predict the rating by weighted globalMean
	weightSum := 0.0
	weightRating := 0.0
	for _, otherId := range candidateSet.candidates[0:numNeighbors] {
		weightSum += knn.sims[leftId][otherId]
		weightRating += knn.sims[leftId][otherId] * knn.ratings[otherId][rightId]
		if knn.tpe == centered {
			weightRating -= knn.sims[leftId][otherId] * knn.means[otherId]
		} else if knn.tpe == baseline {
			weightRating -= knn.sims[leftId][otherId] * knn.bias[otherId]
		}
	}
	prediction := weightRating / weightSum
	if knn.tpe == centered {
		prediction += knn.means[leftId]
	} else if knn.tpe == baseline {
		prediction += knn.bias[leftId]
	}
	return prediction
}

func (knn *KNN) Fit(trainSet TrainSet, options ...OptionSetter) {
	// Setup options
	knn.option = Option{
		sim:       MSD,
		userBased: true,
		k:         40, // the (max) number of neighbors to take into account for aggregation
		minK:      1,  // The minimum number of neighbors to take into account for aggregation.
		// If there are not enough neighbors, the prediction is set the global
		// globalMean of all interactionRatings
	}
	for _, setter := range options {
		setter(&knn.option)
	}
	// Set global globalMean for new users (items)
	knn.trainSet = trainSet
	knn.globalMean = trainSet.GlobalMean()
	// Retrieve user (item) ratings
	if knn.option.userBased {
		knn.ratings = trainSet.UserRatings()
		knn.sims = newNanMatrix(trainSet.UserCount(), trainSet.UserCount())
	} else {
		knn.ratings = trainSet.ItemRatings()
		knn.sims = newNanMatrix(trainSet.ItemCount(), trainSet.ItemCount())
	}
	// Retrieve user (item) mean
	if knn.tpe == centered {
		knn.means = make([]float64, len(knn.ratings))
		for i := range knn.means {
			sum, count := 0.0, 0.0
			for j := range knn.ratings[i] {
				if !math.IsNaN(knn.ratings[i][j]) {
					sum += knn.ratings[i][j]
					count++
				}
			}
			knn.means[i] = sum / count
		}
	} else if knn.tpe == baseline {
		baseLine := NewBaseLine()
		baseLine.Fit(trainSet, options...)
		if knn.option.userBased {
			knn.bias = baseLine.userBias
		} else {
			knn.bias = baseLine.itemBias
		}
	}
	// Pairwise similarity
	for leftId, leftRatings := range knn.ratings {
		for rightId, rightRatings := range knn.ratings {
			if leftId != rightId {
				if math.IsNaN(knn.sims[leftId][rightId]) {
					ret := knn.option.sim(leftRatings, rightRatings)
					if !math.IsNaN(ret) {
						knn.sims[leftId][rightId] = ret
						knn.sims[rightId][leftId] = ret
					}
				}
			}
		}
	}
}
