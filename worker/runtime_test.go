package worker

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/oneRole/queue"
)

type lookupOnlyJetStream struct {
	jetstream.JetStream
	lookups []consumerLookup
}

type consumerLookup struct {
	stream string
	name   string
}

func (client *lookupOnlyJetStream) Consumer(_ context.Context, stream, name string) (jetstream.Consumer, error) {
	client.lookups = append(client.lookups, consumerLookup{stream: stream, name: name})
	return &inertJetStreamConsumer{}, nil
}

type inertJetStreamConsumer struct {
	jetstream.Consumer
}

func TestRuntimeLooksUpPreprovisionedDurablesWhenProvisioningDisabled(t *testing.T) {
	config := DefaultRuntimeConfig("worker-prod-1")
	config.ProvisionStreams = false
	config.ProvisionConsumers = false
	client := &lookupOnlyJetStream{}
	runtime := &Runtime{config: config, dependencies: RuntimeDependencies{JetStream: client}}

	if err := runtime.ensureConsumers(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := []consumerLookup{
		{stream: queue.StreamApplications, name: "application-packet-prepare-v1"},
		{stream: queue.StreamApplications, name: "application-batch-fanout-v1"},
		{stream: queue.StreamApplications, name: "application-submit-v1"},
		{stream: queue.StreamApplications, name: "application-submit-api-v1"},
		{stream: queue.StreamApplications, name: "application-submit-browser-v1"},
		{stream: queue.StreamApplications, name: "application-verify-v1"},
		{stream: queue.StreamApplications, name: "application-reconcile-v1"},
		{stream: queue.StreamApplications, name: "application-retry-v1"},
		{stream: queue.StreamApplications, name: "application-dlq-v1"},
		{stream: queue.StreamDiscovery, name: "discovery-source-sync-v1"},
	}
	if len(client.lookups) != len(want) || len(runtime.runners) != len(want) {
		t.Fatalf("lookups=%d runners=%d, want %d each", len(client.lookups), len(runtime.runners), len(want))
	}
	for index := range want {
		if client.lookups[index] != want[index] {
			t.Errorf("lookup[%d]=%+v, want %+v", index, client.lookups[index], want[index])
		}
	}
	if subject, ok := queue.SubjectForEventType(queue.EventApplicationSubmitRequested); !ok || subject != queue.SubjectApplicationSubmit {
		t.Fatalf("runtime subject contract changed: subject=%q ok=%v", subject, ok)
	}
}
