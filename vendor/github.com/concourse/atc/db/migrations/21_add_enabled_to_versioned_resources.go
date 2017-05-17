package migrations

import "github.com/BurntSushi/migration"

func AddEnabledToBuilds(tx migration.LimitedTx) error {
	_, err := tx.Exec(`ALTER TABLE versioned_resources ADD COLUMN enabled boolean NOT NULL default true`)
	if err != nil {
		return err
	}

	return nil
}
