package controller

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	demov1 "kubebuilder-demo/api/v1"
)

const (
	MyDeploymentFinalizer = "finalizer.mydeployment.demo.exmaple.org"
)

// MyDeploymentReconciler reconciles a MyDeployment object
type MyDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=demo.my.domain,resources=mydeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=demo.my.domain,resources=mydeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.my.domain,resources=mydeployments/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// user: Modify the Reconcile function to compare the state specified by
// the MyDeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
func (r *MyDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	obj := &demov1.MyDeployment{}
	if err := r.Client.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !obj.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, obj)
	}
	return r.reconcile(ctx, obj)
}

func (r *MyDeploymentReconciler) reconcile(ctx context.Context, obj *demov1.MyDeployment) (ctrl.Result, error) {
	controllerutil.AddFinalizer(obj, MyDeploymentFinalizer)
	return r.reconcileReplicas(ctx, obj)
}

func LabelSelectorToMap(labelSelector *metav1.LabelSelector) (map[string]string, error) {
	if labelSelector == nil {
		return nil, fmt.Errorf("labelSelector is nil")
	}
	result := make(map[string]string)

	for key, value := range labelSelector.MatchLabels {
		result[key] = value
	}

	return result, nil
}

func (r *MyDeploymentReconciler) reconcileReplicas(ctx context.Context, obj *demov1.MyDeployment) (ctrl.Result, error) {
	pods := v1.PodList{}
	labels, err := LabelSelectorToMap(obj.Spec.LabelSelector)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Client.List(ctx, &pods, client.InNamespace(obj.Namespace), client.MatchingLabels(labels)); err != nil {
		return ctrl.Result{}, err
	}

	var filteredPods []v1.Pod
	for _, pod := range pods.Items {
		// skip already deleted pod
		if pod.DeletionTimestamp.IsZero() {
			filteredPods = append(filteredPods, pod)
		}
	}

	replicas := *obj.Spec.Replicas
	replicasDiff := replicas - int64(len(filteredPods))
	if replicasDiff > 0 {
		for i := 0; i < int(replicasDiff); i++ {
			// create pods
			pod := r.GeneratePod(ctx, obj)
			if err := r.Client.Create(ctx, pod); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		for i := 0; i < int(-replicasDiff); i++ {
			pod := filteredPods[i]
			// delete additional pod
			if err := r.Client.Delete(ctx, &pod); err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	// alter 10 seconds re control pod replicas
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *MyDeploymentReconciler) GeneratePod(_ context.Context, obj *demov1.MyDeployment) *v1.Pod {
	podTemplate := obj.Spec.Template
	podTemplate.ObjectMeta.GenerateName = obj.Name
	podTemplate.ObjectMeta.Namespace = obj.Namespace
	podTemplate.ObjectMeta.Labels = obj.Spec.LabelSelector.MatchLabels
	pod := &v1.Pod{
		ObjectMeta: podTemplate.ObjectMeta,
		Spec:       podTemplate.Spec,
	}
	_ = controllerutil.SetOwnerReference(obj, pod, r.Scheme)
	return pod
}

func (r *MyDeploymentReconciler) reconcileDelete(ctx context.Context, obj *demov1.MyDeployment) (ctrl.Result, error) {
	pods := v1.PodList{}
	labels, err := LabelSelectorToMap(obj.Spec.LabelSelector)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Client.List(ctx, &pods, client.InNamespace(obj.Namespace), client.MatchingLabels(labels)); err != nil {
		return ctrl.Result{}, err
	}
	var filteredPods []v1.Pod
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp.IsZero() {
			filteredPods = append(filteredPods, pod)
		}
	}
	if len(filteredPods) > 0 {
		log.FromContext(ctx).Info("pods has not delete wait 10 second", "deploy", obj.Namespace+"/"+obj.Name)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}
	if controllerutil.ContainsFinalizer(obj, MyDeploymentFinalizer) {
		controllerutil.RemoveFinalizer(obj, MyDeploymentFinalizer)
		if err := r.Client.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MyDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1.MyDeployment{}).
		Owns(&v1.Pod{}).
		Complete(r)
}
