package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// clientset "k8s.io/client-go/kubernetes"
	clientset "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog"
)

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func main() {
	klog.InitFlags(nil)

	var kubeconfig string
	var leaseLockName string
	var leaseLockNamespace string
	var leaseDuration int
	var renewDeadline int
	var retryPeriod int
	var id string
	var port string
	var leaderID string

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&id, "id", uuid.New().String(), "the holder identity name")
	flag.StringVar(&leaseLockName, "lockName", "", "the lock resource name")
	flag.StringVar(&leaseLockNamespace, "lockNamespace", "", "the lock resource namespace")
	flag.IntVar(&leaseDuration, "leaseDuration", 4, "time (seconds) that non-leader candidates will wait to force acquire leadership")
	flag.IntVar(&renewDeadline, "renewDeadline", 2, "time (seconds) that the acting leader will retry refreshing leadership before giving up")
	flag.IntVar(&retryPeriod, "retryPeriod", 1, "time (seconds) LeaderElector candidates should wait between tries of actions")
	flag.StringVar(&port, "port", "", "If non-empty, stand up a simple webserver that reports the leader state")
	flag.Parse()

	if leaseLockName == "" {
		klog.Fatal("unable to get lease lock resource name (missing lease-lock-name flag).")
	}
	if leaseLockNamespace == "" {
		klog.Fatal("unable to get lease lock resource namespace (missing lease-lock-namespace flag).")
	}

	// leader election uses the Kubernetes API by writing to a
	// lock object, which can be a LeaseLock object (preferred),
	// a ConfigMap, or an Endpoints (deprecated) object.
	// Conflicting writes are detected and each client handles those actions
	// independently.
	config, err := buildConfig(kubeconfig)
	if err != nil {
		klog.Fatal(err)
	}
	client := clientset.NewForConfigOrDie(config)

	// run := func(ctx context.Context) {
	// 	// complete your controller loop here
	// 	klog.Info("Controller loop...")

	// 	select {}
	// }

	// use a Go context so we can tell the leaderelection code when we
	// want to step down
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// listen for interrupts or the Linux SIGTERM signal and cancel
	// our context, which the leader election code will observe and
	// step down
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		klog.Info("Received termination, signaling shutdown")
		cancel()
	}()

	// we use the Lease lock type since edits to Leases are less common
	// and fewer objects in the cluster watch "all Leases".
	lock := &resourcelock.ConfigMapLock{
		ConfigMapMeta: metav1.ObjectMeta{
			Name:      leaseLockName,
			Namespace: leaseLockNamespace,
		},
		Client: client,
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}
	// lock := &resourcelock.LeaseLock{
	// 	LeaseMeta: metav1.ObjectMeta{
	// 		Name:      leaseLockName,
	// 		Namespace: leaseLockNamespace,
	// 	},
	// 	Client: client.CoordinationV1(),
	// 	LockConfig: resourcelock.ResourceLockConfig{
	// 		Identity: id,
	// 	},
	// }

	if len(port) > 0 {
		webHandler := func(res http.ResponseWriter, req *http.Request) {
			data, err := json.Marshal(leaderID)
			if err != nil {
				res.WriteHeader(http.StatusInternalServerError)
				res.Write([]byte(err.Error()))
				return
			}
			res.WriteHeader(http.StatusOK)
			res.Write(data)
		}
		klog.Infof("Registering web handler at %s", port)
		http.HandleFunc("/", webHandler)
		go http.ListenAndServe(":"+port, nil)
	}

	klog.Infof("commencing leader election for %s", id)
	// start the leader election code loop
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock: lock,
		// IMPORTANT: you MUST ensure that any code you have that
		// is protected by the lease must terminate **before**
		// you call cancel. Otherwise, you could have a background
		// loop still running and another process could
		// get elected before your background loop finished, violating
		// the stated goal of the lease.
		ReleaseOnCancel: true,
		LeaseDuration:   time.Duration(leaseDuration) * time.Second,
		RenewDeadline:   time.Duration(renewDeadline) * time.Second,
		RetryPeriod:     time.Duration(retryPeriod) * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				// we're notified when we start - this is where you would
				// usually put your code
				klog.Infof("%s started leading", id)
				// run(ctx)
			},
			OnStoppedLeading: func() {
				// we can do cleanup here
				klog.Infof("%s stopped leading", id)
				// os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				// we're notified when new leader elected
				leaderID = identity
				if identity == id {
					// I just got the lock
					return
				}
				klog.Infof("%s elected new leader", leaderID)
			},
		},
	})

}
