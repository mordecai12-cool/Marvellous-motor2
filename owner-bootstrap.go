package main

import "database/sql"

// Built-in accounts use bcrypt hashes in every environment. The migration runs
// once per account so password changes survive subsequent starts and deploys.
const deploymentOwnerEmail = "emmanlemichel2019@gmail.com"
const deploymentOwnerPasswordHash = "$2a$10$XWtBgE78XyFfWCi54rGbDezANfrxsKGrIoXfgLxSZoV73sYHgYCuW"
const deploymentSecondOwnerPasswordHash = "$2a$10$vxhTMTIhQd67aXnxptvz/O3MqNfgnr6qjnxDkc79Ol6LAh27G42Q."

func seedDeploymentOwner(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS built_in_admin_setup(email TEXT PRIMARY KEY,initialized_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	for _, a := range []struct{ name, email, hash string }{{"Emmanuel Michel", deploymentOwnerEmail, deploymentOwnerPasswordHash}, {"Marvellous Mordecai Otiti", deploymentSecondOwnerEmail, deploymentSecondOwnerPasswordHash}} {
		result, e := tx.Exec("INSERT OR IGNORE INTO built_in_admin_setup(email) VALUES(?)", a.email)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n == 0 {
			continue
		}
		result, e = tx.Exec("UPDATE admins SET name=?,email=?,password=?,role='super_admin',status='approved' WHERE lower(email)=?", a.name, a.email, a.hash, a.email)
		if e != nil {
			return e
		}
		n, e = result.RowsAffected()
		if e != nil {
			return e
		}
		if n == 0 {
			if _, e = tx.Exec("INSERT INTO admins(name,email,password,role,status) VALUES(?,?,?,'super_admin','approved')", a.name, a.email, a.hash); e != nil {
				return e
			}
		}
	}
	return tx.Commit()
}
