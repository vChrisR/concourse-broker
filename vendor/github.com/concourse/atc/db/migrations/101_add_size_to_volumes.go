package migrations

import "github.com/BurntSushi/migration"

func AddSizeToVolumes(tx migration.LimitedTx) error {
	_, err := tx.Exec(`
		ALTER TABLE volumes
		ADD COLUMN size integer default 0;
`)
	return err
}
