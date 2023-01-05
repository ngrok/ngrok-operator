package controllers

import "sigs.k8s.io/controller-runtime/pkg/controller"

// NonLeaderElectedController is a controller wrapper that does not need to be leader elected
type NonLeaderElectedController struct {
	controller.Controller
}

// NeedLoeaderElection helps NonLeaderElectedController implement the manager.LeaderElectionRunnable interface.
func (c NonLeaderElectedController) NeedLeaderElection() bool {
	return false
}
