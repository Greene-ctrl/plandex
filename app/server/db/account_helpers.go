package db

import (
	"context"
	"fmt"
	"log"
	"os"
	shared "plandex-shared"

	"github.com/jmoiron/sqlx"
)

type CreateAccountResult struct {
	User  *User
	OrgId string
	Token string
}

func CreateAccount(name, email, emailVerificationId string, tx *sqlx.Tx) (*CreateAccountResult, error) {
	isLocalMode := (os.Getenv("GOENV") == "development" && os.Getenv("LOCAL_MODE") == "1")
	// create user
	user, err := CreateUser(name, email, tx)

	if err != nil {
		return nil, fmt.Errorf("error creating user: %v", err)
	}

	userId := user.Id
	domain := user.Domain

	// create auth token
	token, authTokenId, err := CreateAuthToken(userId, tx)

	if err != nil {
		return nil, fmt.Errorf("error creating auth token: %v", err)
	}

	// skipping email verification in local mode
	if !isLocalMode {
		// update email verification with user and auth token ids
		_, err = tx.Exec("UPDATE email_verifications SET user_id = $1, auth_token_id = $2 WHERE id = $3", userId, authTokenId, emailVerificationId)

		if err != nil {
			return nil, fmt.Errorf("error updating email verification: %v", err)
		}
	}

	// add to org matching domain if one exists and auto add domain users is true for that org
	orgId, err := AddToOrgForDomain(domain, userId, tx)

	if err != nil {
		return nil, fmt.Errorf("error adding user to org for domain: %v", err)
	}

	return &CreateAccountResult{
		User:  user,
		OrgId: orgId,
		Token: token,
	}, nil
}

func EnsureAdminUserAndOrg() error {
	var userCount int
	err := Conn.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return fmt.Errorf("EnsureAdminUserAndOrg: error checking user count: %v", err)
	}

	if userCount > 0 {
		return nil
	}

	log.Println("EnsureAdminUserAndOrg: No users found. Seeding default admin user and organization...")

	err = WithTx(context.Background(), "seed admin", func(tx *sqlx.Tx) error {
		// Create admin user
		user, err := CreateUser("Admin", "admin@plandex.ai", tx)
		if err != nil {
			return fmt.Errorf("EnsureAdminUserAndOrg: error creating seed user: %v", err)
		}

		// Create default org
		org, err := CreateOrg(&shared.CreateOrgRequest{
			Name: "Plandex",
		}, user.Id, &user.Domain, tx)
		if err != nil {
			return fmt.Errorf("EnsureAdminUserAndOrg: error creating seed org: %v", err)
		}

		log.Printf("EnsureAdminUserAndOrg: Successfully seeded Admin user (%s) and Org (%s)\n", user.Id, org.Id)
		return nil
	})

	return err
}
