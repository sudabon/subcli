package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sudabon/subcli/internal/memory"
	"github.com/sudabon/subcli/internal/provider"
)

func newMemoryCmd(deps *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "リポジトリローカルの共有メモリ（.subcli/memory）を管理する",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMemoryStatus(deps)
		},
	}

	cmd.AddCommand(
		newMemoryStatusCmd(deps),
		newMemoryInitCmd(deps),
		newMemoryAddCmd(deps),
		newMemoryShowCmd(deps),
		newMemoryListCmd(deps),
		newMemoryPathCmd(deps),
		newMemorySyncCmd(deps),
		newMemoryRmCmd(deps),
		newMemoryImportCmd(deps),
	)
	return cmd
}

func openRepoMemory(deps *Deps) (*memory.Store, error) {
	cwd, err := deps.Getwd()
	if err != nil {
		return nil, fmt.Errorf("カレントディレクトリの取得に失敗しました: %w", err)
	}
	return memory.Open(cwd)
}

func newMemoryStatusCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "共有メモリの初期化状態と .gitignore 設定を確認する",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMemoryStatus(deps)
		},
	}
}

func runMemoryStatus(deps *Deps) error {
	store, err := openRepoMemory(deps)
	if err != nil {
		return err
	}
	st, err := store.Status()
	if err != nil {
		return err
	}

	fmt.Fprintf(deps.Stdout, "repo_root:   %s\n", st.RepoRoot)
	fmt.Fprintf(deps.Stdout, "memory_dir:  %s\n", st.MemoryDir)
	fmt.Fprintf(deps.Stdout, "initialized: %t\n", st.Initialized)
	fmt.Fprintf(deps.Stdout, "gitignored:  %t\n", st.Gitignored)
	if st.Initialized {
		fmt.Fprintf(deps.Stdout, "files:       %d\n", st.FileCount)
		fmt.Fprintf(deps.Stdout, "main_bytes:  %d\n", st.MainSizeBytes)
		fmt.Fprintf(deps.Stdout, "projections: claude=%t agy=%t (codex=inline)\n", st.ClaudeProjectionSet, st.AgyProjectionSet)
	} else {
		fmt.Fprintf(deps.Stdout, "\n`subcli memory init` を実行すると .subcli/memory/MEMORY.md を作成し .gitignore へ .subcli/ を追記します。\n")
	}
	return nil
}

func newMemoryInitCmd(deps *Deps) *cobra.Command {
	var skipGitignore bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "リポジトリルートに .subcli/memory を初期化し .gitignore へ追記する",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := openRepoMemory(deps)
			if err != nil {
				return err
			}
			res, err := store.Init(memory.InitOptions{SkipGitignore: skipGitignore})
			if err != nil {
				return err
			}
			if res.CreatedMemory {
				fmt.Fprintf(deps.Stdout, "作成しました: %s\n", res.MemoryFile)
			} else {
				fmt.Fprintf(deps.Stdout, "既存の共有メモリを確認しました: %s\n", res.MemoryFile)
			}
			if res.UpdatedGitignore {
				fmt.Fprintf(deps.Stdout, ".gitignore に .subcli/ を追記しました（チーム非共有）\n")
			} else if !skipGitignore {
				fmt.Fprintf(deps.Stdout, ".gitignore には既に .subcli/ が設定されています\n")
			}
			fmt.Fprintf(deps.Stdout, "プロバイダ向け投影を同期しました（subcli open claude|codex|agy で自動利用されます）\n")
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipGitignore, "no-gitignore", false, ".gitignore への .subcli/ 追記をスキップする")
	return cmd
}

func newMemoryAddCmd(deps *Deps) *cobra.Command {
	var (
		topic         string
		skipGitignore bool
	)
	cmd := &cobra.Command{
		Use:   "add \"<text>\"",
		Short: "リポジトリ共有メモリへエントリを追記する（シークレット自動マスク）",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := openRepoMemory(deps)
			if err != nil {
				return err
			}
			res, err := store.AddEntry(args[0], topic, deps.Now(), memory.InitOptions{SkipGitignore: skipGitignore})
			if err != nil {
				return err
			}
			if res.UpdatedGitignore {
				fmt.Fprintf(deps.Stderr, ".gitignore に .subcli/ を追記しました\n")
			}
			if res.Redacted {
				fmt.Fprintf(deps.Stderr, "警告: 機密情報らしい文字列を検出したためマスクして保存しました\n")
			}
			fmt.Fprintf(deps.Stdout, "追記しました: %s\n", res.TargetFile)
			return nil
		},
	}
	cmd.Flags().StringVar(&topic, "topic", "", "詳細トピックファイル名（例: architecture -> .subcli/memory/architecture.md）")
	cmd.Flags().BoolVar(&skipGitignore, "no-gitignore", false, "自動初期化時に .gitignore へ追記しない")
	return cmd
}

func newMemoryShowCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show [topic]",
		Short: "共有メモリ（MEMORY.md または指定トピック）の内容を表示する",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := openRepoMemory(deps)
			if err != nil {
				return err
			}
			var content string
			if len(args) == 0 {
				content, err = store.ReadMain()
			} else {
				content, err = store.ReadTopic(args[0])
			}
			if err != nil {
				return err
			}
			fmt.Fprint(deps.Stdout, content)
			if !strings.HasSuffix(content, "\n") {
				fmt.Fprintln(deps.Stdout)
			}
			return nil
		},
	}
}

func newMemoryListCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "共有メモリ配下の Markdown ファイル一覧を表示する",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := openRepoMemory(deps)
			if err != nil {
				return err
			}
			files, err := store.List()
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(deps.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "FILE\tBYTES\tUPDATED")
			for _, f := range files {
				fmt.Fprintf(tw, "%s\t%d\t%s\n", f.Name, f.Size, f.ModTime.Format("2006-01-02 15:04:05"))
			}
			return tw.Flush()
		},
	}
}

func newMemoryPathCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "共有メモリディレクトリ（<repo-root>/.subcli/memory）のパスを表示する",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := openRepoMemory(deps)
			if err != nil {
				return err
			}
			fmt.Fprintln(deps.Stdout, store.MemoryDir)
			return nil
		},
	}
}

func newMemorySyncCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "MEMORY.md から各プロバイダ向け投影ファイルを再生成する",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := openRepoMemory(deps)
			if err != nil {
				return err
			}
			if err := store.Sync(); err != nil {
				return err
			}
			fmt.Fprintf(deps.Stdout, "投影を同期しました: %s\n", store.ProjectionsDir)
			return nil
		},
	}
}

func newMemoryRmCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <topic>",
		Short: "指定したトピックファイルを削除し索引と投影を更新する",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := openRepoMemory(deps)
			if err != nil {
				return err
			}
			if err := store.RemoveTopic(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(deps.Stdout, "トピック %q を削除しました\n", args[0])
			return nil
		},
	}
}

func newMemoryImportCmd(deps *Deps) *cobra.Command {
	var (
		sourceDir     string
		skipGitignore bool
	)
	cmd := &cobra.Command{
		Use:   "import <claude|codex>",
		Short: "既存エージェントのローカル Markdown メモリを読み取り専用で取り込む",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			provName := strings.ToLower(strings.TrimSpace(args[0]))
			store, err := openRepoMemory(deps)
			if err != nil {
				return err
			}

			src := strings.TrimSpace(sourceDir)
			if src == "" {
				homeDir, homeErr := os.UserHomeDir()
				if homeErr != nil {
					return fmt.Errorf("ホームディレクトリの取得に失敗しました: %w", homeErr)
				}
				env := deps.Environ()
				lookup := func(key string) string {
					val, _ := provider.LookupEnvSlice(env, key)
					return val
				}
				src, err = memory.DefaultImportDir(provName, store.RepoRoot, homeDir, lookup)
				if err != nil {
					return err
				}
			}

			res, err := store.ImportMarkdownDir(src, provName, deps.Now(), memory.InitOptions{SkipGitignore: skipGitignore})
			if err != nil {
				return err
			}
			if res.UpdatedGitignore {
				fmt.Fprintf(deps.Stderr, ".gitignore に .subcli/ を追記しました\n")
			}
			if res.Redacted {
				fmt.Fprintf(deps.Stderr, "警告: 機密情報らしい文字列を検出したためマスクして取り込みました\n")
			}
			fmt.Fprintf(deps.Stdout, "%s から %d 件の Markdown ファイルを取り込みました: %s\n", provName, len(res.ImportedFiles), res.TargetFile)
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceDir, "source", "", "取り込み元メモリディレクトリを明示指定する")
	cmd.Flags().BoolVar(&skipGitignore, "no-gitignore", false, "自動初期化時に .gitignore へ追記しない")
	return cmd
}
