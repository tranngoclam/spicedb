package datastore

import (
	"errors"

	pb "github.com/authzed/spicedb/pkg/REDACTEDapi/api"
)

// Publicly facing errors
var (
	ErrNamespaceNotFound  = errors.New("unable to find namespace")
	ErrPreconditionFailed = errors.New("unable to satisfy write precondition")
)

// TupleDatastore represents tuple access for a single namespace.
type TupleDatastore interface {
	// WriteTuples takes a list of existing tuples that must exist, and a list of tuple
	// mutations and applies it to the datastore for the specified namespace.
	WriteTuples(preconditions []*pb.RelationTuple, mutations []*pb.RelationTupleUpdate) (uint64, error)

	// QueryTuples creates a builder for reading tuples from the datastore.
	QueryTuples(namespace string, revision uint64) TupleQuery

	// Revision gets the currently replicated revision for this datastore.
	Revision() (uint64, error)
}

// TupleQuery is a builder for constructing tuple queries.
type TupleQuery interface {
	// WithObjectID adds an object ID filter to the query.
	WithObjectID(objectID string) TupleQuery

	// WithRelation adds a relation filter to the query.
	WithRelation(relation string) TupleQuery

	// WithUserset adds a userset filter to the query.
	WithUserset(userset *pb.ObjectAndRelation) TupleQuery

	// Execute runs the tuple query and returns a result iterator.
	Execute() (TupleIterator, error)
}

// TupleIterator is an iterator over matched tuples.
type TupleIterator interface {
	// Next returns the next tuple in the result set.
	Next() *pb.RelationTuple

	// After receiving a nil response, the caller must check for an error.
	Err() error

	// Close cancels the query and closes any open connections.
	Close()
}

// NamespaceDatastore defines an interface for communicating with the persistent data
// of the server.
type NamespaceDatastore interface {
	// WriteNamespace takes a proto namespace definition and persists it,
	// returning the version of the namespace that was created.
	WriteNamespace(newConfig *pb.NamespaceDefinition) (uint64, error)

	// ReadNamespace reads a namespace definition and version and returns it if found.
	ReadNamespace(nsName string) (*pb.NamespaceDefinition, uint64, error)
}
