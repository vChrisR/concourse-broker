package migrations

import "github.com/BurntSushi/migration"

func AddInputsDeterminedToBuilds(tx migration.LimitedTx) error {
	_, err := tx.Exec(`
		ALTER TABLE builds ADD COLUMN inputs_determined bool NOT NULL DEFAULT false
	`)

	if err != nil {
		return err
	}

	_, err = tx.Exec(`
			UPDATE builds
			SET inputs_determined = true
	`)

	return err
}
