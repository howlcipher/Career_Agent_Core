package graph

import (
	"context"
	"fmt"
)

// State represents a node in the graph
type State string

// Node represents a function that processes the state and returns the next state
type Node[T any] func(ctx context.Context, state T) (State, error)

// Graph is a stateful pipeline execution graph
type Graph[T any] struct {
	nodes map[State]Node[T]
}

// NewGraph creates a new directed graph pipeline
func NewGraph[T any]() *Graph[T] {
	return &Graph[T]{
		nodes: make(map[State]Node[T]),
	}
}

// AddNode adds a new state processing node to the graph
func (g *Graph[T]) AddNode(state State, node Node[T]) {
	g.nodes[state] = node
}

// Run executes the graph starting from initialState until it reaches a state that is not in the graph or an error occurs
func (g *Graph[T]) Run(ctx context.Context, initialState State, state T) error {
	currentState := initialState

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		node, exists := g.nodes[currentState]
		if !exists {
			// Terminal state reached (either END, DONE, or unknown state)
			return nil
		}

		nextState, err := node(ctx, state)
		if err != nil {
			return fmt.Errorf("error in state %s: %w", currentState, err)
		}

		if nextState == "" {
			return nil // End execution if empty state returned
		}

		currentState = nextState
	}
}
