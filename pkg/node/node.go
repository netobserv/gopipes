// Package node provides functionalities to create nodes and interconnect them.
// A Node is a function container that can be connected via channels to other nodes.
// A node can send data to multiple nodes, and receive data from multiple nodes.
package node

import (
	"context"
	"reflect"

	"github.com/netobserv/gopipes/pkg/node/internal/connect"
)

// StartFunc is a function that receives a writable channel as unique argument, and sends
// value to that channel during an indefinite amount of time.
type StartFunc[OUT any] func(out chan<- OUT)

// StartFuncCtx is a StartFunc that also receives a context as a first argument. If the passed
// context is cancelled via the ctx.Done() function, the implementer function should end,
// so the cancel will be propagated to the later nodes.
type StartFuncCtx[OUT any] func(ctx context.Context, out chan<- OUT)

// MiddleFunc is a function that receives a readable channel as first argument,
// and a writable channel as second argument.
// It must process the inputs from the input channel until it's closed.
type MiddleFunc[IN, OUT any] func(in <-chan IN, out chan<- OUT)

// TerminalFunc is a function that receives a readable channel as unique argument.
// It must process the inputs from the input channel until it's closed.
type TerminalFunc[IN any] func(out <-chan IN)

// TODO: OutType and InType methods are candidates for deprecation

// Sender is any node that can send data to another node: node.Start and node.Middle
type Sender[OUT any] interface {
	// SendsTo connect a sender with a group of receivers
	SendsTo(...Receiver[OUT])
	// OutType returns the inner type of the Sender's output channel
	OutType() reflect.Type
}

// Receiver is any node that can receive data from another node: node.Middle and node.Terminal
type Receiver[IN any] interface {
	isStarted() bool
	start()
	joiner() *connect.Joiner[IN]
	// InType returns the inner type of the Receiver's input channel
	InType() reflect.Type
}

// Start nodes are the starting points of a graph. This is, all the nodes that bring information
// from outside the graph: e.g. because they generate them or because they acquire them from an
// external source like a Web Service.
// A graph must have at least one Start node.
// A Start node must have at least one output node.
type Start[OUT any] struct {
	outs    []Receiver[OUT]
	fun     StartFuncCtx[OUT]
	outType reflect.Type
}

func (s *Start[OUT]) SendsTo(outputs ...Receiver[OUT]) {
	s.outs = append(s.outs, outputs...)
}

// OutType is deprecated. It will be removed in future versions.
func (s *Start[OUT]) OutType() reflect.Type {
	return s.outType
}

// Middle is any intermediate node that receives data from another node, processes/filters it,
// and forwards the data to another node.
// An Middle node must have at least one output node.
type Middle[IN, OUT any] struct {
	outs    []Receiver[OUT]
	inputs  connect.Joiner[IN]
	started bool
	fun     MiddleFunc[IN, OUT]
	outType reflect.Type
	inType  reflect.Type
}

func (i *Middle[IN, OUT]) joiner() *connect.Joiner[IN] {
	return &i.inputs
}

func (i *Middle[IN, OUT]) isStarted() bool {
	return i.started
}

func (s *Middle[IN, OUT]) SendsTo(outputs ...Receiver[OUT]) {
	s.outs = append(s.outs, outputs...)
}

func (m *Middle[IN, OUT]) OutType() reflect.Type {
	return m.outType
}

func (m *Middle[IN, OUT]) InType() reflect.Type {
	return m.inType
}

// Terminal is any node that receives data from another node and does not forward it to another node,
// but can process it and send the results to outside the graph (e.g. memory, storage, web...)
type Terminal[IN any] struct {
	inputs  connect.Joiner[IN]
	started bool
	fun     TerminalFunc[IN]
	done    chan struct{}
	inType  reflect.Type
}

func (i *Terminal[IN]) joiner() *connect.Joiner[IN] {
	return &i.inputs
}

func (t *Terminal[IN]) isStarted() bool {
	return t.started
}

// Done returns a channel that is closed when the Terminal node has ended its processing. This
// is, when all its inputs have been also closed. Waiting for all the Terminal nodes to finish
// allows blocking the execution until all the data in the graph has been processed and all the
// previous stages have ended
func (t *Terminal[IN]) Done() <-chan struct{} {
	return t.done
}

func (m *Terminal[IN]) InType() reflect.Type {
	return m.inType
}

// AsStart wraps a StartFunc into a Start node.
// Deprecated. Use AsStart or AsStartCtx
func AsInit[OUT any](fun StartFunc[OUT]) *Start[OUT] {
	return AsStart(fun)
}

// AsStart wraps a StartFunc into a Start node.
func AsStart[OUT any](fun StartFunc[OUT]) *Start[OUT] {
	return AsStartCtx(func(_ context.Context, out chan<- OUT) {
		fun(out)
	})
}

// AsStartCtx wraps a StartFuncCtx into a Start node.
func AsStartCtx[OUT any](fun StartFuncCtx[OUT]) *Start[OUT] {
	var out OUT
	return &Start[OUT]{
		fun:     fun,
		outType: reflect.TypeOf(out),
	}
}

// AsMiddle wraps an MiddleFunc into an Middle node.
func AsMiddle[IN, OUT any](fun MiddleFunc[IN, OUT], opts ...Option) *Middle[IN, OUT] {
	var in IN
	var out OUT
	options := getOptions(opts...)
	return &Middle[IN, OUT]{
		inputs:  connect.NewJoiner[IN](options.channelBufferLen),
		fun:     fun,
		inType:  reflect.TypeOf(in),
		outType: reflect.TypeOf(out),
	}
}

// AsTerminal wraps a TerminalFunc into a Terminal node.
func AsTerminal[IN any](fun TerminalFunc[IN], opts ...Option) *Terminal[IN] {
	var i IN
	options := getOptions(opts...)
	return &Terminal[IN]{
		inputs: connect.NewJoiner[IN](options.channelBufferLen),
		fun:    fun,
		done:   make(chan struct{}),
		inType: reflect.TypeOf(i),
	}
}

// Start the function wrapped in the Start node. Either this method or StartCtx should be invoked
// for all the start nodes of the same graph, so the graph can properly start and finish.
func (i *Start[OUT]) Start() {
	i.StartCtx(context.TODO())
}

// StartCtx starts the function wrapped in the Start node, allow passing a context that can be
// used by the wrapped function. Either this method or Start should be invoked
// for all the start nodes of the same graph, so the graph can properly start and finish.
func (i *Start[OUT]) StartCtx(ctx context.Context) {
	if len(i.outs) == 0 {
		panic("Start node should have outputs")
	}
	joiners := make([]*connect.Joiner[OUT], 0, len(i.outs))
	for _, out := range i.outs {
		joiners = append(joiners, out.joiner())
		if !out.isStarted() {
			out.start()
		}
	}
	forker := connect.Fork(joiners...)
	go func() {
		i.fun(ctx, forker.Sender())
		forker.Close()
	}()
}

func (i *Middle[IN, OUT]) start() {
	if len(i.outs) == 0 {
		panic("Middle node should have outputs")
	}
	i.started = true
	joiners := make([]*connect.Joiner[OUT], 0, len(i.outs))
	for _, out := range i.outs {
		joiners = append(joiners, out.joiner())
		if !out.isStarted() {
			out.start()
		}
	}
	forker := connect.Fork(joiners...)
	go func() {
		i.fun(i.inputs.Receiver(), forker.Sender())
		forker.Close()
	}()
}

func (t *Terminal[IN]) start() {
	t.started = true
	go func() {
		t.fun(t.inputs.Receiver())
		close(t.done)
	}()
}

func getOptions(opts ...Option) creationOptions {
	options := defaultOptions
	for _, opt := range opts {
		opt(&options)
	}
	return options
}
