package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leonardomarzeuski/symbion/internal/schema"
)

func currentProject(cwd string) (string, error) {
	loadedSchema, err := schema.Load(filepath.Join(cwd, schema.DefaultFilename))
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%s not found; run symbion init or symbion scan first", schema.DefaultFilename)
		}
		return "", err
	}

	return loadedSchema.Project, nil
}
