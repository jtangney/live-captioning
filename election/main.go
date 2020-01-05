package main

import (
	"context"
	"flag"
	"fmt"
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
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog"
)

func buildConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func main() {
	klog.InitFlags(nil)

	var lockName string
	var lockNamespace string
	var leaseDuration int
	var renewDeadline int
	var retryPeriod int
	var id string
	var port int
	var leaderID string

	flag.StringVar(&id, "id", uuid.New().String(), "This pod's participant ID for leader election")
	flag.IntVar(&port, "electionPort", 4040, "Required. Send HTTP leader election notifications to this port")
	flag.StringVar(&lockName, "lockName", "", "the lock resource name")
	flag.StringVar(&lockNamespace, "lockNamespace", "default", "the lock resource namespace")
	flag.IntVar(&leaseDuration, "leaseDuration", 4, "time (seconds) that non-leader candidates will wait to force acquire leadership")
	flag.IntVar(&renewDeadline, "renewDeadline", 2, "time (seconds) that the acting leader will retry refreshing leadership before giving up")
	flag.IntVar(&retryPeriod, "retryPeriod", 1, "time (seconds) LeaderElector candidates should wait between tries of actions")
	flag.Parse()

	if port <= 0 {
		klog.Fatal("Missing --electionPort flag")
	}
	if lockName == "" {
		klog.Fatal("Missing --lockName flag.")
	}
	if lockNamespace == "" {
		klog.Fatal("Missing --lockNamespace flag.")
	}

	// leader election uses the Kubernetes API by writing to a
	// lock object, which can be a LeaseLock object (preferred),
	// a ConfigMap, or an Endpoints (deprecated) object.
	// Conflicting writes are detected and each client handles those actions
	// independently.
	config, err := buildConfig()
	if err != nil {
		klog.Fatal(err)
	}
	client := clientset.NewForConfigOrDie(config)

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
			Name:      lockName,
			Namespace: lockNamespace,
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

	klog.Infof("Commencing leader election for candidate %s", id)
	listenerRootURL := fmt.Sprintf("http://localhost:%d", port)
	startURL := fmt.Sprintf("%s/start", listenerRootURL)
	stopURL := fmt.Sprintf("%s/stop", listenerRootURL)
	klog.Infof("Will notify listener at %s of leader changes", listenerRootURL)

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
				// this pod now the leader! Notify listener
				klog.Infof("Notifying %s started leading", id)
				resp, err := http.Get(startURL)
				defer resp.Body.Close()
				if err != nil {
					klog.Errorf("Failed to notify leader of start: %v", err)
				}
			},
			OnStoppedLeading: func() {
				// this pod stopped leading! Notify listener
				klog.Infof("Notifying %s stopped leading", id)
				resp, err := http.Get(stopURL)
				defer resp.Body.Close()
				if err != nil {
					klog.Errorf("Failed to notify leader of stop: %v", err)
				}
			},
			OnNewLeader: func(identity string) {
				// the leader has changed
				leaderID = identity
				klog.Infof("%s elected as new leader", identity)
			},
		},
	})

}
