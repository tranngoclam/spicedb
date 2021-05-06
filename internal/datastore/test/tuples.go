package test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/authzed/spicedb/internal/datastore"
	"github.com/authzed/spicedb/internal/testfixtures"
	pb "github.com/authzed/spicedb/pkg/REDACTEDapi/api"
	"github.com/authzed/spicedb/pkg/tuple"
)

const (
	testUserNamespace     = "test/user"
	testResourceNamespace = "test/resource"
	testReaderRelation    = "reader"
	ellipsis              = "..."
)

func TestSimple(t *testing.T, tester DatastoreTester) {
	testCases := []int{1, 2, 4, 32, 1024}

	for _, numTuples := range testCases {
		t.Run(strconv.Itoa(numTuples), func(t *testing.T) {
			require := require.New(t)

			ds, err := tester.New(0, disableGC)
			require.NoError(err)

			setupDatastore(ds, require)

			tRequire := testfixtures.TupleChecker{Require: require, DS: ds}

			var testTuples []*pb.RelationTuple

			ctx := context.Background()

			var lastRevision uint64
			for i := 0; i < numTuples; i++ {
				resourceName := fmt.Sprintf("resource%d", i)
				userName := fmt.Sprintf("user%d", i)

				newTuple := makeTestTuple(resourceName, userName)
				testTuples = append(testTuples, newTuple)

				writtenAt, err := ds.WriteTuples(
					ctx,
					nil,
					[]*pb.RelationTupleUpdate{tuple.Create(newTuple)},
				)
				require.NoError(err)
				require.Greater(writtenAt, lastRevision)

				tRequire.TupleExists(ctx, newTuple, writtenAt)
				tRequire.TupleExists(ctx, newTuple, writtenAt+100)
				tRequire.NoTupleExists(ctx, newTuple, writtenAt-1)

				lastRevision = writtenAt
			}

			for _, tupleToFind := range testTuples {
				// Check that we can find the tuple a number of ways
				q := ds.QueryTuples(tupleToFind.ObjectAndRelation.Namespace, lastRevision)

				queries := []datastore.TupleQuery{
					q.WithObjectID(tupleToFind.ObjectAndRelation.ObjectId),
					q.WithUserset(tupleToFind.User.GetUserset()),
					q.WithObjectID(tupleToFind.ObjectAndRelation.ObjectId).WithRelation(tupleToFind.ObjectAndRelation.Relation),
					q.WithObjectID(tupleToFind.ObjectAndRelation.ObjectId).WithUserset(tupleToFind.User.GetUserset()),
					q.WithRelation(tupleToFind.ObjectAndRelation.Relation).WithUserset(tupleToFind.User.GetUserset()),
				}
				for _, query := range queries {
					iter, err := query.Execute(ctx)
					require.NoError(err)
					tRequire.VerifyIteratorResults(iter, tupleToFind)
				}
			}

			// Check that we can find the group of tuples too
			q := ds.QueryTuples(testTuples[0].ObjectAndRelation.Namespace, lastRevision)

			queries := []datastore.TupleQuery{
				q,
				q.WithRelation(testTuples[0].ObjectAndRelation.Relation),
			}
			for _, query := range queries {
				iter, err := query.Execute(ctx)
				require.NoError(err)
				tRequire.VerifyIteratorResults(iter, testTuples...)
			}

			// Try some bad queries
			badQueries := []datastore.TupleQuery{
				q.WithObjectID("fakeobjectid"),
				q.WithUserset(&pb.ObjectAndRelation{
					Namespace: "test/user",
					ObjectId:  "fakeuser",
					Relation:  ellipsis,
				}),
			}
			for _, badQuery := range badQueries {
				iter, err := badQuery.Execute(ctx)
				require.NoError(err)
				tRequire.VerifyIteratorResults(iter)
			}

			// Delete the first tuple
			deletedAt, err := ds.WriteTuples(
				ctx,
				nil,
				[]*pb.RelationTupleUpdate{tuple.Delete(testTuples[0])},
			)
			require.NoError(err)

			// Delete it AGAIN (idempotent delete) and make sure there's no error
			_, err = ds.WriteTuples(
				ctx,
				nil,
				[]*pb.RelationTupleUpdate{tuple.Delete(testTuples[0])},
			)
			require.NoError(err)

			// Verify it can still be read at the old revision
			tRequire.TupleExists(ctx, testTuples[0], deletedAt-1)

			// Verify that it does not show up at the new revision
			tRequire.NoTupleExists(ctx, testTuples[0], deletedAt)
			alreadyDeletedIter, err := ds.QueryTuples(
				testTuples[0].ObjectAndRelation.Namespace,
				deletedAt,
			).Execute(ctx)
			require.NoError(err)
			tRequire.VerifyIteratorResults(alreadyDeletedIter, testTuples[1:]...)
		})
	}
}

func TestPreconditions(t *testing.T, tester DatastoreTester) {
	require := require.New(t)

	ds, err := tester.New(0, disableGC)
	require.NoError(err)

	setupDatastore(ds, require)

	first := makeTestTuple("first", "owner")
	second := makeTestTuple("second", "owner")

	ctx := context.Background()

	_, err = ds.WriteTuples(
		ctx,
		[]*pb.RelationTuple{first},
		[]*pb.RelationTupleUpdate{tuple.Create(second)},
	)
	require.True(errors.Is(err, datastore.ErrPreconditionFailed))

	_, err = ds.WriteTuples(ctx, nil, []*pb.RelationTupleUpdate{tuple.Create(first)})
	require.NoError(err)

	_, err = ds.WriteTuples(
		ctx,
		[]*pb.RelationTuple{first},
		[]*pb.RelationTupleUpdate{tuple.Create(second)},
	)
	require.NoError(err)
}

func TestRevisionFuzzing(t *testing.T, tester DatastoreTester) {
	require := require.New(t)

	fuzzingRange := 100 * time.Millisecond

	ds, err := tester.New(fuzzingRange, disableGC)
	require.NoError(err)

	setupDatastore(ds, require)

	ctx := context.Background()

	// Create some revisions
	tpl := makeTestTuple("first", "owner")
	for i := 0; i < 10; i++ {
		_, err = ds.WriteTuples(ctx, nil, []*pb.RelationTupleUpdate{tuple.Touch(tpl)})
		require.NoError(err)
	}

	// Get the new now revision
	nowRevision, err := ds.SyncRevision(ctx)
	require.NoError(err)
	require.GreaterOrEqual(nowRevision, uint64(0))

	foundAnotherRevision := false
	for start := time.Now(); time.Since(start) < 20*time.Millisecond; {
		testRevision, err := ds.Revision(ctx)
		require.NoError(err)
		require.LessOrEqual(testRevision, nowRevision)
		if testRevision < nowRevision {
			foundAnotherRevision = true
			break
		}
	}

	require.True(foundAnotherRevision)

	// Let the fuzzing window expire
	time.Sleep(fuzzingRange)

	// Now we should ONLY get the now revision
	for start := time.Now(); time.Since(start) < 10*time.Millisecond; {
		testRevision, err := ds.Revision(ctx)
		require.NoError(err)
		require.Equal(nowRevision, testRevision)
	}
}

func TestInvalidReads(t *testing.T, tester DatastoreTester) {
	t.Run("revision expiration", func(t *testing.T) {
		testGCDuration := 10 * time.Millisecond

		require := require.New(t)

		ds, err := tester.New(0, testGCDuration)
		require.NoError(err)

		setupDatastore(ds, require)

		ctx := context.Background()

		// Check that we get an error when there are no revisions
		err = ds.CheckRevision(ctx, 0)
		require.Equal(datastore.ErrInvalidRevision, err)

		newTuple := makeTestTuple("one", "one")
		firstWrite, err := ds.WriteTuples(
			ctx,
			nil,
			[]*pb.RelationTupleUpdate{tuple.Create(newTuple)},
		)
		require.NoError(err)

		// Check that we can read at the just written revision
		err = ds.CheckRevision(ctx, firstWrite)
		require.NoError(err)

		// Wait the duration required to allow the revision to expire
		time.Sleep(testGCDuration * 2)

		// Check that we can still read the just written revision even though it's expired
		err = ds.CheckRevision(ctx, firstWrite)
		require.NoError(err)

		// Write another tuple which will allow the first revision to expire
		nextWrite, err := ds.WriteTuples(
			ctx,
			nil,
			[]*pb.RelationTupleUpdate{tuple.Touch(newTuple)},
		)
		require.NoError(err)

		// Check that we can read at the just written revision
		err = ds.CheckRevision(ctx, nextWrite)
		require.NoError(err)

		// Check that we can no longer read the old revision (now allowed to expire)
		err = ds.CheckRevision(ctx, firstWrite)
		require.Equal(datastore.ErrInvalidRevision, err)

		// Check that we can't read a revision that's ahead of the latest
		err = ds.CheckRevision(ctx, nextWrite+1)
		require.Equal(datastore.ErrInvalidRevision, err)

		// Check that we can't read a revision that's WAY ahead of the latest
		err = ds.CheckRevision(ctx, nextWrite+1000000)
		require.Equal(datastore.ErrInvalidRevision, err)
	})
}