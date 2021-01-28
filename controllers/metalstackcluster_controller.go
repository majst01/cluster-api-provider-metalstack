/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	api "github.com/metal-stack/cluster-api-provider-metalstack/api/v1alpha3"
	metalgo "github.com/metal-stack/metal-go"
	"github.com/metal-stack/metal-lib/pkg/tag"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/pointer"
	capi "sigs.k8s.io/cluster-api/api/v1alpha3"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	MetalStackClusterFinalizer = "metalstackcluster.infrastructure.cluster.x-k8s.io"
)

// MetalStackClusterReconciler reconciles a MetalStackCluster object
type MetalStackClusterReconciler struct {
	Client           client.Client
	Log              logr.Logger
	MetalStackClient MetalStackClient
}

func NewMetalStackClusterReconciler(metalClient MetalStackClient, mgr manager.Manager) *MetalStackClusterReconciler {
	return &MetalStackClusterReconciler{
		Client:           mgr.GetClient(),
		Log:              ctrl.Log.WithName("controllers").WithName("MetalStackCluster"),
		MetalStackClient: metalClient,
	}
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metalstackclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=metalstackclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

func (r *MetalStackClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.MetalStackCluster{}).
		Watches(
			&source.Kind{Type: &capi.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.ClusterToInfrastructureMapFunc(api.GroupVersion.WithKind("MetalStackCluster")),
			}).
		Complete(r)
}

// Reconcile reconciles MetalStackCluster resource
func (r *MetalStackClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, err error) {
	ctx := context.Background()
	logger := r.Log.WithValues("MetalStackCluster", req.NamespacedName)

	logger.Info("Starting MetalStackCluster reconcilation")

	// Fetch the MetalStackCluster.
	metalCluster := &api.MetalStackCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, metalCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("fetch MetalStackCluster: %w", err)
	}

	// Persist any changes to MetalStackCluster.
	h, err := patch.NewHelper(metalCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("init patch helper: %w", err)
	}
	defer func() {
		if e := h.Patch(ctx, metalCluster); e != nil {
			if err != nil {
				err = errors.Wrap(e, err.Error())
			}
			err = fmt.Errorf("patch: %w", e)
		}
	}()

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, metalCluster.ObjectMeta)
	if err != nil {
		clusterErr := capierrors.InvalidConfigurationClusterError
		metalCluster.Status.FailureReason = &clusterErr
		metalCluster.Status.FailureMessage = pointer.StringPtr("Unable to get OwnerCluster")
		return ctrl.Result{}, fmt.Errorf("get OwnerCluster: %w", err)
	}
	if cluster == nil {
		logger.Info("Waiting for cluster controller to set OwnerRef to MetalStackCluster")
		return requeueWithDelay, nil
	}

	if util.IsPaused(cluster, metalCluster) {
		logger.Info("reconcilation is paused for this object")
		return requeueWithDelay, nil
	}

	if !metalCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, logger, cluster, metalCluster)
	}

	return r.reconcile(ctx, logger, metalCluster)
}

func (r *MetalStackClusterReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, cluster *capi.Cluster, metalCluster *api.MetalStackCluster) (ctrl.Result, error) {
	logger.Info("Deleting MetalStackCluster")

	// Check if there's still active machines in Cluster
	machineCount, err := r.countMachines(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to count machines: %w", err)
	}
	if machineCount > 0 {
		// Delete machines
		logger.Info("Deleting Cluster's Machines")
		if err := r.deleteMachines(ctx, cluster); err != nil {
			return ctrl.Result{}, fmt.Errorf("Failed to delete machines: %w", err)
		}

		return requeueWithSmallDelay, nil
	}

	// Delete firewall
	logger.Info("Deleting Firewall")
	if err := r.deleteFirewall(ctx, metalCluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to delete Firewall: %w", err)
	}

	// Wait until all IPs are freed
	resp, err := r.MetalStackClient.IPList()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to list IPs: %w", err)
	}
	if len(resp.IPs) > 0 {
		logger.Info("Waiting until all IPs are freed")
		return requeueWithSmallDelay, nil

	}

	// Delete network
	logger.Info("Deleting Cluster network")
	if _, err := r.MetalStackClient.NetworkFree(*metalCluster.Spec.PrivateNetworkID); err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to free network: %w", err)
	}

	controllerutil.RemoveFinalizer(metalCluster, MetalStackClusterFinalizer)
	logger.Info("Successfully deleted MetalStackCluster")

	return ctrl.Result{}, nil
}

func (r *MetalStackClusterReconciler) reconcile(ctx context.Context, logger logr.Logger, metalCluster *api.MetalStackCluster) (ctrl.Result, error) {
	controllerutil.AddFinalizer(metalCluster, MetalStackClusterFinalizer)

	// Allocate network.
	if metalCluster.Spec.PrivateNetworkID == nil {
		if err := r.allocateNetwork(metalCluster); err != nil {
			logger.Info(err.Error() + ": requeueing")
			return requeueWithDelay, nil
		}
	}

	// Allocate IP for API server
	if !metalCluster.Status.ControlPlaneIPAllocated {
		if err := r.allocateControlPlaneIP(logger, metalCluster); err != nil {
			return requeueInstantly, nil
		}
	}

	// Create firewall
	if !metalCluster.Status.FirewallReady {
		err := r.createFirewall(logger, metalCluster)
		if err != nil {
			logger.Info(err.Error() + ": requeueing")
			return requeueWithDelay, nil
		}
		logger.Info("Cluster firewall is created")
		metalCluster.Status.FirewallReady = true
	}

	metalCluster.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *MetalStackClusterReconciler) allocateNetwork(metalCluster *api.MetalStackCluster) error {
	resp, err := r.MetalStackClient.NetworkAllocate(&metalgo.NetworkAllocateRequest{
		Description: metalCluster.Name,
		Labels:      map[string]string{tag.ClusterID: metalCluster.Name},
		Name:        metalCluster.Spec.Partition,
		PartitionID: metalCluster.Spec.Partition,
		ProjectID:   metalCluster.Spec.ProjectID,
	})
	if err != nil {
		return err
	}

	metalCluster.Spec.PrivateNetworkID = resp.Network.ID

	return nil
}

func (r *MetalStackClusterReconciler) allocateControlPlaneIP(logger logr.Logger, metalCluster *api.MetalStackCluster) error {
	if _, err := r.MetalStackClient.IPAllocate(&metalgo.IPAllocateRequest{
		Description: "",
		Name:        metalCluster.Name + "api-server-IP",
		Networkid:   "internet-vagrant-lab",
		Projectid:   metalCluster.Spec.ProjectID,
		IPAddress:   metalCluster.Spec.ControlPlaneEndpoint.Host,
		Type:        "",
		Tags:        []string{},
	}); err != nil {
		logger.Info(fmt.Sprintf("Failed to allocate Control Plane IP %s", metalCluster.Spec.ControlPlaneEndpoint.Host))
		return err
	}

	metalCluster.Status.ControlPlaneIPAllocated = true
	logger.Info(fmt.Sprintf("Control Plane IP %s allocated", metalCluster.Spec.ControlPlaneEndpoint.Host))

	return nil
}

// todo: Ask metal-API for an available external network IP (partition id empty -> destinationprefix: 0.0.0.0/0)
func (r *MetalStackClusterReconciler) createFirewall(logger logr.Logger, metalCluster *api.MetalStackCluster) error {
	if metalCluster.Spec.Firewall.DefaultNetworkID == nil {
		return fmt.Errorf("Firewall.DefaultNetworkID")
	}
	if metalCluster.Spec.Firewall.Image == nil {
		return fmt.Errorf("Firewall.Image")
	}
	if metalCluster.Spec.Firewall.Size == nil {
		return fmt.Errorf("Firewall.Size")
	}
	if metalCluster.Spec.PrivateNetworkID == nil {
		return fmt.Errorf("PrivateNetworkID")
	}

	machineCreateReq := metalgo.MachineCreateRequest{
		Description:   metalCluster.Name + " created by Cluster API provider MetalStack",
		Name:          metalCluster.Name,
		Hostname:      metalCluster.Name + "-firewall",
		Size:          *metalCluster.Spec.Firewall.Size,
		Project:       metalCluster.Spec.ProjectID,
		Partition:     metalCluster.Spec.Partition,
		Image:         *metalCluster.Spec.Firewall.Image,
		SSHPublicKeys: metalCluster.Spec.Firewall.SSHKeys,
		Networks:      toNetworks(*metalCluster.Spec.Firewall.DefaultNetworkID, *metalCluster.Spec.PrivateNetworkID),
		UserData:      "",
		Tags:          []string{},
	}

	// Set machine ID if it's set in firewall config
	if pid, err := metalCluster.Spec.Firewall.ParsedProviderID(); err == nil {
		logger.Info(fmt.Sprintf("Deploy Firewall on machine: %s", pid))
		machineCreateReq.UUID = pid
	}

	resp, err := r.MetalStackClient.FirewallCreate(&metalgo.FirewallCreateRequest{
		MachineCreateRequest: machineCreateReq,
	})
	if err != nil {
		return err
	}

	metalCluster.Spec.Firewall.SetProviderID(*resp.Firewall.ID)
	return nil
}

func (r *MetalStackClusterReconciler) countMachines(ctx context.Context, cluster *capi.Cluster) (count int, err error) {
	machines := capi.MachineList{}
	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{
			capi.ClusterLabelName: cluster.Name,
		}),
	}

	if r.Client.List(ctx, &machines, listOptions...) != nil {
		return 0, fmt.Errorf("Failed to list machines: %w", err)
	}

	return len(machines.Items), nil
}

func (r *MetalStackClusterReconciler) deleteMachines(ctx context.Context, cluster *capi.Cluster) error {
	machine := capi.Machine{}
	deleteOptions := []client.DeleteAllOfOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{
			capi.ClusterLabelName: cluster.Name,
		}),
	}
	if err := r.Client.DeleteAllOf(ctx, &machine, deleteOptions...); err != nil {
		return err
	}

	return nil
}

func (r *MetalStackClusterReconciler) deleteFirewall(ctx context.Context, metalCluster *api.MetalStackCluster) error {
	providerId, err := metalCluster.Spec.Firewall.ParsedProviderID()
	if err != nil {
		return fmt.Errorf("failed to parse providerID of Firewall: %w", err)
	}

	_, err = r.MetalStackClient.MachineDelete(providerId)
	return err
}
