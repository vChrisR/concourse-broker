package migrations

import "github.com/BurntSushi/migration"

func AddStepLocationToContainers(tx migration.LimitedTx) error {
	_, err := tx.Exec(`
		ALTER TABLE containers ADD COLUMN step_location integer DEFAULT 0;
	`)

	return err
}
