package migrations

import "github.com/BurntSushi/migration"

func AddResourceTypesToWorkers(tx migration.LimitedTx) error {
	_, err := tx.Exec(`ALTER TABLE workers ADD COLUMN resource_types text`)
	if err != nil {
		return err
	}

	return nil
}
