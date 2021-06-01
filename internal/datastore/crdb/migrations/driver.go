package migrations

import (
	"context"
	"fmt"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

const errUnableToInstantiate = "unable to instantiate CRDBDriver: %w"

const postgresMissingTableErrorCode = "42P01"

type CRDBDriver struct {
	db *pgx.Conn
}

func NewCRDBDriver(url string) (*CRDBDriver, error) {
	connConfig, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf(errUnableToInstantiate, err)
	}

	// connConfig.Logger = zerologadapter.NewLogger(log.Logger)

	db, err := pgx.ConnectConfig(context.Background(), connConfig)
	if err != nil {
		return nil, fmt.Errorf(errUnableToInstantiate, err)
	}

	return &CRDBDriver{db}, nil
}

func (apd *CRDBDriver) Version() (string, error) {
	var loaded string

	if err := apd.db.QueryRow(context.Background(), "SELECT version_num from schema_version").Scan(&loaded); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == postgresMissingTableErrorCode {
			return "", nil
		}
		return "", fmt.Errorf("unable to load alembic revision: %w", err)
	}

	return loaded, nil
}

func (apd *CRDBDriver) WriteVersion(version, replaced string) error {
	result, err := apd.db.Exec(context.Background(), "UPDATE schema_version SET version_num=$1 WHERE version_num=$2", version, replaced)
	if err != nil {
		return fmt.Errorf("unable to update version row: %w", err)
	}

	updatedCount := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("unable to compute number of rows affected: %w", err)
	}

	if updatedCount != 1 {
		return fmt.Errorf("writing version update affected %d rows, should be 1", updatedCount)
	}

	return nil
}