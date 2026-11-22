package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sudabon/subcli/internal/session"
)

func newSessionsCmd(deps *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "ローカルに保存されたセッション履歴を一覧・詳細表示・削除する",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSessionsList(cmd, deps)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "保存済みセッション一覧を表示する",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSessionsList(cmd, deps)
		},
	}

	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "指定セッションのメタデータ詳細を表示する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionsShow(cmd, deps, args[0])
		},
	}

	rmCmd := &cobra.Command{
		Use:   "rm <id>",
		Short: "指定セッションを削除する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionsRm(cmd, deps, args[0])
		},
	}

	cmd.AddCommand(listCmd, showCmd, rmCmd)
	return cmd
}

func runSessionsList(cmd *cobra.Command, deps *Deps) error {
	store, err := deps.OpenStore()
	if err != nil {
		return err
	}
	defer store.Close()

	records, err := store.List(cmd.Context())
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Fprintln(deps.Stdout, "保存されたセッションはありません。")
		return nil
	}

	tw := tabwriter.NewWriter(deps.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tPROVIDER\tLAST_RUN\tCWD\tPROMPT")
	for _, r := range records {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			r.ID,
			r.Provider,
			r.LastRunAt.Local().Format("2006-01-02 15:04:05"),
			r.Cwd,
			r.PromptPreview,
		)
	}
	return tw.Flush()
}

func runSessionsShow(cmd *cobra.Command, deps *Deps, id string) error {
	store, err := deps.OpenStore()
	if err != nil {
		return err
	}
	defer store.Close()

	rec, err := store.Get(cmd.Context(), id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return fmt.Errorf("セッション %q が見つかりません", id)
		}
		return err
	}

	fmt.Fprintf(deps.Stdout, "id: %s\n", rec.ID)
	fmt.Fprintf(deps.Stdout, "provider: %s\n", rec.Provider)
	fmt.Fprintf(deps.Stdout, "provider_session_id: %s\n", rec.ProviderSessionID)
	fmt.Fprintf(deps.Stdout, "cwd: %s\n", rec.Cwd)
	fmt.Fprintf(deps.Stdout, "created_at: %s\n", rec.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(deps.Stdout, "last_run_at: %s\n", rec.LastRunAt.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(deps.Stdout, "prompt_preview: %s\n", rec.PromptPreview)
	return nil
}

func runSessionsRm(cmd *cobra.Command, deps *Deps, id string) error {
	store, err := deps.OpenStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Delete(cmd.Context(), id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return fmt.Errorf("セッション %q が見つかりません", id)
		}
		return err
	}
	fmt.Fprintf(deps.Stdout, "セッション %s を削除しました。\n", id)
	return nil
}
