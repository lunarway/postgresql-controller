package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"k8s.io/utils/strings/slices"
)

type Extensions = []Extension

func ensureExtensions(ctx context.Context, conn *sql.DB, serviceCredentials, adminCredentials Credentials, extensions Extensions) error {
	if extensionsIsEmpty(extensions) {
		// Nothing to reconcile, extensions are not removed if removed from the extensions page
		return nil
	}

	alreadyAvailableExtensions, err := getInstalledExtensions(ctx, conn, serviceCredentials)
	if err != nil {
		return fmt.Errorf("failed to get installed extensions: %w", err)
	}

	extensionsToInstall, needsReconciliation := extensionsToInstall(extensions, alreadyAvailableExtensions)
	if !needsReconciliation {
		// No work needs to be done, returning
		return nil
	}

	if err := installExtensions(ctx, conn, adminCredentials, serviceCredentials, extensionsToInstall); err != nil {
		return fmt.Errorf("failed to install extensions: %w", err)
	}

	return nil
}

// getInstalledExtensions finds the already enabled extensions in the database
func getInstalledExtensions(ctx context.Context, conn *sql.DB, credentials Credentials) (Extensions, error) {
	// https://www.postgresql.org/docs/current/catalog-pg-extension.html
	rows, err := conn.QueryContext(
		ctx,
		prependSetRole(
			`SELECT extname FROM pg_extension`,
			credentials.User,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list extensions from the database: %w", err)
	}
	defer rows.Close()

	extensions := make(Extensions, 0)
	for rows.Next() {
		var extname string
		err = rows.Scan(&extname)
		if err != nil {
			return nil, fmt.Errorf("failed to read row: %w", err)
		}

		extensions = append(extensions, NewExtension(extname))
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("failed to read rows: %w", err)
	}

	return extensions, nil

}

// extensionsToInstall diffs the existing extensions with the desired list and finds the extensions that needs to be installed
func extensionsToInstall(extensions, alreadyAvailableExtensions Extensions) (Extensions, bool) {
	alreadyAvailable := make([]string, 0, len(alreadyAvailableExtensions))
	for _, e := range alreadyAvailableExtensions {
		alreadyAvailable = append(alreadyAvailable, e.Name)
	}

	toInstall := make(Extensions, 0)
	for _, e := range extensions {
		if !slices.Contains(alreadyAvailable, e.Name) {
			toInstall = append(toInstall, e)
		}
	}

	return toInstall, !extensionsIsEmpty(toInstall)
}

// installExtensions actually enable extensions on the database
func installExtensions(ctx context.Context, conn *sql.DB, adminCredentials, serviceCredentials Credentials, extensionsToInstall []Extension) error {
	for _, e := range extensionsToInstall {
		_, err := conn.ExecContext(
			ctx,
			fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s WITH SCHEMA %s", e.Name, serviceCredentials.Name),
		)
		if err != nil {
			return fmt.Errorf("failed to install: user: %s, db: %s, extension %s: %w", adminCredentials.User, serviceCredentials.Name, e.Name, err)
		}
	}

	return nil
}

// extensionsIsEmpty returns whether a list of extensions has any items
func extensionsIsEmpty(extensions Extensions) bool {
	return len(extensions) == 0
}

// Extension is a reference to a postgresql extension
type Extension struct {
	Name string
}

func NewExtension(name string) Extension {
	return Extension{
		Name: name,
	}
}
