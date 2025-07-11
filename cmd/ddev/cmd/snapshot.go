package cmd

import (
	"bytes"
	"fmt"

	"github.com/ddev/ddev/pkg/ddevapp"
	"github.com/ddev/ddev/pkg/globalconfig"
	"github.com/ddev/ddev/pkg/nodeps"
	"github.com/ddev/ddev/pkg/output"
	"github.com/ddev/ddev/pkg/styles"
	"github.com/ddev/ddev/pkg/util"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

var snapshotAll bool
var snapshotCleanup bool
var snapshotList bool
var snapshotName string
var snapshotRestoreLatest bool

// noConfirm: If true, --yes, we won't stop and prompt before each deletion
var snapshotCleanupNoConfirm bool

// DdevSnapshotCommand provides the snapshot command
var DdevSnapshotCommand = &cobra.Command{
	ValidArgsFunction: ddevapp.GetProjectNamesFunc("all", 0),
	Use:               "snapshot [projectname projectname...]",
	Short:             "Create a database snapshot for one or more projects.",
	Long:              `Uses mariabackup or xtrabackup command to create a database snapshot in the .ddev/db_snapshots folder. These are compatible with server backups using the same tools and can be restored with "ddev snapshot restore".`,
	Example: `ddev snapshot
ddev snapshot --name some_descriptive_name
ddev snapshot --cleanup
ddev snapshot --cleanup -y
ddev snapshot --list
ddev snapshot --all`,
	Run: func(_ *cobra.Command, args []string) {
		apps, err := getRequestedProjects(args, snapshotAll)
		if err != nil {
			util.Failed("Unable to get project(s) %v: %v", args, err)
		}
		if len(apps) > 0 {
			instrumentationApp = apps[0]
		}

		if snapshotList {
			listSnapshots(apps)
			return
		}

		for _, app := range apps {
			if app.Database.Type == nodeps.Postgres && app.Database.Version == nodeps.Postgres9 {
				util.Failed("Snapshots are not supported for postgres:9")
			}
			switch {
			case snapshotCleanup:
				deleteAppSnapshot(app)
			default:
				createAppSnapshot(app)
			}
		}
	},
}

func listSnapshots(apps []*ddevapp.DdevApp) {
	var columns table.Row
	var out bytes.Buffer
	t := table.NewWriter()
	t.SetOutputMirror(&out)
	styles.SetGlobalTableStyle(t, false)

	if len(apps) > 1 {
		columns = append(columns, "Project")
	}
	columns = append(columns, "Snapshot", "Created")

	if !globalconfig.DdevGlobalConfig.SimpleFormatting {
		var colConfig []table.ColumnConfig
		for _, col := range columns {
			colConfig = append(colConfig, table.ColumnConfig{
				Name: fmt.Sprint(col),
			})
		}
		t.SetColumnConfigs(colConfig)
	}
	t.AppendHeader(columns)

	allSnapshots := make(map[string][]ddevapp.Snapshot)
	for _, app := range apps {
		if snapshots, err := app.ListSnapshots(); err != nil {
			util.Failed("Failed to list snapshots %s: %v", app.GetName(), err)
		} else {
			allSnapshots[app.GetName()] = snapshots
			if len(snapshots) > 0 {
				for _, snapshot := range snapshots {
					if len(apps) > 1 {
						t.AppendRow(table.Row{app.GetName(), snapshot.Name, snapshot.Created.Format("2006-01-02")})
					} else {
						t.AppendRow(table.Row{snapshot.Name, snapshot.Created.Format("2006-01-02")})
					}
				}
			} else {
				if len(apps) > 1 {
					t.AppendRow(table.Row{app.GetName(), text.Italic.Sprint("No snapshots"), ""})
				} else {
					t.AppendRow(table.Row{text.Italic.Sprint("No snapshots"), ""})
				}
			}
		}
	}

	t.Render()
	output.UserOut.WithField("raw", allSnapshots).Println(out.String())
}

func createAppSnapshot(app *ddevapp.DdevApp) {
	// If the database is omitted, do not snapshot
	omittedContainers := app.GetOmittedContainers()
	if nodeps.ArrayContainsString(omittedContainers, "db") {
		util.Warning("Database is omitted for project %s, skipping snapshot", app.GetName())
		return
	}

	appStatus, _ := app.SiteStatus()
	// If the app is not running, then start it to create a snapshot.
	if appStatus != ddevapp.SiteRunning {
		util.Warning("Project %s is %s, starting it to create a snapshot", app.GetName(), appStatus)
		if err := app.Start(); err != nil {
			util.Failed("Failed to start %s: %v", app.GetName(), err)
		}
	}
	// If there is an error from Snapshot, show a warning message
	// allow the command to continue, there may be other snapshots needed
	if snapshotNameOutput, err := app.Snapshot(snapshotName); err != nil {
		errorMsg := util.ColorizeText("Failed to snapshot %s: %v", "red")
		util.Warning(errorMsg, app.GetName(), err)
	} else {
		util.Success("Created database snapshot %s", snapshotNameOutput)
		util.Success("Restore this snapshot with 'ddev snapshot restore %s'", snapshotNameOutput)
	}
	// Return the app to its previous state, stopped or paused.
	if appStatus == ddevapp.SiteStopped {
		if err := app.Stop(false, false); err != nil {
			util.Failed("Failed to stop %s: %v", app.GetName(), err)
		}
	}
	if appStatus == ddevapp.SitePaused {
		if err := app.Pause(); err != nil {
			util.Failed("Failed to pause %s: %v", app.GetName(), err)
		}
	}
}

func deleteAppSnapshot(app *ddevapp.DdevApp) {
	var snapshotsToDelete []string
	var prompt string
	var err error

	if !snapshotCleanupNoConfirm {
		if snapshotName == "" {
			prompt = fmt.Sprintf("OK to delete all snapshots of %s.", app.GetName())
		} else {
			prompt = fmt.Sprintf("OK to delete the snapshot '%s' of project '%s'", snapshotName, app.GetName())
		}
		if !util.Confirm(prompt) {
			return
		}
	}

	if snapshotName == "" {
		snapshotsToDelete, err = app.ListSnapshotNames()
		if err != nil {
			util.Failed("Failed to detect snapshots %s: %v", app.GetName(), err)
		}
	} else {
		snapshotsToDelete = append(snapshotsToDelete, snapshotName)
	}

	for _, snapshotToDelete := range snapshotsToDelete {
		if err := app.DeleteSnapshot(snapshotToDelete); err != nil {
			util.Failed("Failed to delete snapshot %s: %v", app.GetName(), err)
		}
	}
}

func init() {
	DdevSnapshotCommand.Flags().BoolVarP(&snapshotAll, "all", "a", false, "Snapshot all projects. Will start the project if it is stopped or paused")
	DdevSnapshotCommand.Flags().BoolVarP(&snapshotList, "list", "l", false, "List snapshots")
	DdevSnapshotCommand.Flags().BoolVarP(&snapshotCleanup, "cleanup", "C", false, "Cleanup snapshots")
	DdevSnapshotCommand.Flags().BoolVarP(&snapshotCleanupNoConfirm, "yes", "y", false, "Yes - skip confirmation prompt")
	DdevSnapshotCommand.Flags().StringVarP(&snapshotName, "name", "n", "", "provide a name for the snapshot")
	RootCmd.AddCommand(DdevSnapshotCommand)
}
