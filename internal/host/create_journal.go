package host

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func validCreateKind(kind CreateKind) bool { return kind == CreateContainer || kind == CreateNetwork }

func (j *journal) creationOwner(ctx context.Context, o Ownership) error {
	var count int
	err := j.db.QueryRowContext(ctx, `SELECT count(*) FROM instances i JOIN installations p ON p.module_id=i.module_id WHERE i.id=? AND i.epoch=? AND p.storage_key=?`, o.InstanceID, o.EpochID, o.InstallationID).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("create submission ownership mismatch")
	}
	return nil
}
func (j *journal) SubmitCreate(ctx context.Context, o Ownership, kind CreateKind, identity string) error {
	if !validCreateKind(kind) || identity == "" {
		return invalid("invalid create submission identity")
	}
	if err := j.creationOwner(ctx, o); err != nil {
		return err
	}
	// Never UPSERT or retry a submitted operation, even when its caller received
	// an error. Its single deterministic name and ownership bind any late result.
	// No SDK error, including a definite-looking HTTP failure, is an implemented
	// negative-result fence. Keep the reservation until a labeled object is seen.
	result, err := j.db.ExecContext(ctx, `INSERT INTO create_submissions(instance,kind,identity,state,object_id) SELECT ?,?,?,'SUBMITTED','' FROM instances WHERE id=? AND cleanup!='COMPLETE' ON CONFLICT(instance,kind) DO NOTHING`, o.InstanceID, kind, identity, o.InstanceID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrCreateAlreadySubmitted
	}
	return nil
}
func (j *journal) LookupCreate(ctx context.Context, o Ownership, kind CreateKind) (CreateRecord, bool, error) {
	if !validCreateKind(kind) {
		return CreateRecord{}, false, invalid("invalid create submission kind")
	}
	if err := j.creationOwner(ctx, o); err != nil {
		return CreateRecord{}, false, err
	}
	var record CreateRecord
	err := j.db.QueryRowContext(ctx, `SELECT identity,state,object_id FROM create_submissions WHERE instance=? AND kind=?`, o.InstanceID, kind).Scan(&record.Identity, &record.State, &record.ObjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return CreateRecord{}, false, nil
	}
	if err != nil {
		return CreateRecord{}, false, err
	}
	if record.Identity == "" || (record.State != CreateSubmitted && record.State != CreateObserved) || (record.State == CreateObserved && record.ObjectID == "") {
		return CreateRecord{}, false, errors.New("invalid durable create submission state")
	}
	return record, true, nil
}
func (j *journal) ObserveCreated(ctx context.Context, o Ownership, kind CreateKind, identity, id string) error {
	if id == "" {
		return errors.New("created object identity is missing")
	}
	record, found, err := j.LookupCreate(ctx, o, kind)
	if err != nil {
		return err
	}
	if !found || record.Identity != identity {
		return errors.New("created object has no matching submitted operation")
	}
	if record.State == CreateObserved && record.ObjectID != id {
		return errors.New("created object does not match the resolved submitted operation")
	}
	result, err := j.db.ExecContext(ctx, `UPDATE create_submissions SET state='OBSERVED',object_id=? WHERE instance=? AND kind=? AND identity=? AND (state='SUBMITTED' OR (state='OBSERVED' AND object_id=?))`, id, o.InstanceID, kind, identity, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("created object resolution conflicted")
	}
	return nil
}

func requireCreateResolved(ctx context.Context, j CreateJournal, o Ownership, kind CreateKind, identity string) error {
	if j == nil {
		return prerequisite(errors.New("durable Docker create journal is not bound"))
	}
	record, found, err := j.LookupCreate(ctx, o, kind)
	if err != nil {
		return err
	}
	if !found {
		return nil
	} // No request was submitted under the write-ahead contract.
	if record.Identity != identity {
		return errors.New("create cleanup identity mismatch")
	}
	if record.State == CreateSubmitted {
		return fmt.Errorf("%w: %s", ErrCreateUnresolved, kind)
	}
	return nil
}
func (j *journal) unresolvedCreates(ctx context.Context, id string) error {
	var count int
	if err := j.db.QueryRowContext(ctx, `SELECT count(*) FROM create_submissions WHERE instance=? AND state!='OBSERVED'`, id).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return ErrCreateUnresolved
	}
	return nil
}

// Only this recoverable class may leave startup reconciliation pending while
// the supervisor serves and continues polling. Other host failures fail closed.
func unresolvedCleanupOnly(err error) bool {
	if err == nil || !errors.Is(err, ErrCreateUnresolved) {
		return false
	}
	var allowed func(error) bool
	allowed = func(current error) bool {
		if multiple, ok := current.(interface{ Unwrap() []error }); ok {
			for _, child := range multiple.Unwrap() {
				if !allowed(child) {
					return false
				}
			}
			return true
		}
		if single, ok := current.(interface{ Unwrap() error }); ok {
			return allowed(single.Unwrap())
		}
		return current == ErrCreateUnresolved || current == ErrCleanupIncomplete
	}
	return allowed(err)
}

// Backfill only pre-ledger states whose old dispatch path could have submitted
// a create. ALLOCATING/ALLOCATED preflight failures are not fabricated requests.
func (j *journal) initializeCreateJournal(ctx context.Context) error {
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var initialized int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM metadata WHERE key='create_journal_v1'").Scan(&initialized); err != nil {
		return err
	}
	if initialized != 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT lease FROM instances WHERE cleanup!='COMPLETE' AND (state IN ('CREATING','CREATED','STARTING','STARTED') OR (state='FAILED' AND reason IN ('CONTAINER_CREATE_FAILED','CONTAINER_START_FAILED','JOURNAL_WRITE_FAILED')))`)
	if err != nil {
		return err
	}
	var leases []Lease
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var l Lease
		if err = json.Unmarshal([]byte(raw), &l); err != nil {
			rows.Close()
			return err
		}
		leases = append(leases, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, l := range leases {
		// A legacy ID is only a lookup hint until current ownership labels match.
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO create_submissions VALUES(?,?,?,'SUBMITTED',?)`, l.Spec.InstanceID, CreateContainer, containerName(l.Ownership), l.Ownership.ContainerID); err != nil {
			return err
		}
		if l.Spec.InitialEgress == EgressPublicInternet {
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO create_submissions VALUES(?,?,?,'SUBMITTED','')`, l.Spec.InstanceID, CreateNetwork, containerName(l.Ownership)+"-net"); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO metadata(key,value) VALUES('create_journal_v1','1')"); err != nil {
		return err
	}
	return tx.Commit()
}
