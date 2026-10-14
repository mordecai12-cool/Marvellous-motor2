package main

import "database/sql"

// The user-configured owner is created once on a fresh database, in any mode.
// Password verification uses bcrypt; startup never resets an existing account.
const deploymentOwnerEmail = "emmanlemichel2019@gmail.com"
const deploymentOwnerPasswordHash = "$2a$10$8PNVbu.6QGudNnzaGgMnY.uVBKA7xxspD5jQGYtOLO3oPoBYoYfXS"

func seedDeploymentOwner(db *sql.DB) error {
	_, err := db.Exec("INSERT INTO admins(name,email,password,role,status) SELECT 'Platform Owner',?,?,'super_admin','approved' WHERE NOT EXISTS(SELECT 1 FROM admins WHERE lower(email)=?)", deploymentOwnerEmail, deploymentOwnerPasswordHash, deploymentOwnerEmail)
	return err
}
