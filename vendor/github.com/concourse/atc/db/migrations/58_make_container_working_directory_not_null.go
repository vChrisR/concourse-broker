package migrations

import "github.com/BurntSushi/migration"

func MakeContainerWorkingDirectoryNotNull(tx migration.LimitedTx) error {
	_, err := tx.Exec(`
		ALTER TABLE containers ALTER COLUMN working_directory SET DEFAULT '';
	`)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
	  UPDATE containers SET working_directory = '' WHERE working_directory IS null;
  `)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
	  ALTER TABLE containers ALTER COLUMN working_directory SET NOT NULL;
  `)
	return err
}
