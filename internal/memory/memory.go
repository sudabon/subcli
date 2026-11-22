package memory

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/provider"
)

const (
	// SubcliDirName is the repository-local directory managed by subcli (gitignored).
	SubcliDirName = ".subcli"
	// MemoryDirName is the subdirectory holding canonical Markdown memory files.
	MemoryDirName = "memory"
	// ProjectionsDirName is the subdirectory holding generated provider projections.
	ProjectionsDirName = "projections"
	// MainFileName is the canonical shared memory and index file.
	MainFileName = "MEMORY.md"

	// MaxProjectionBytes caps generated projection files safely below provider
	// per-file limits (e.g. agy 24KB, claude 25KB).
	MaxProjectionBytes = 20000
	// MaxInlineArgBytes caps inline developer instructions passed via CLI args.
	MaxInlineArgBytes = 12000
)

var (
	// ErrNotInitialized is returned when .subcli/memory/MEMORY.md has not been created yet.
	ErrNotInitialized = errors.New("共有メモリが初期化されていません（`subcli memory init` を実行してください）")
	// ErrInvalidTopic is returned when a topic name violates naming rules.
	ErrInvalidTopic = errors.New("トピック名が不正です（英数字で始まり、英数字・ハイフン・アンダースコア・ドットのみ 64 文字以内、MEMORY 以外）")
	// ErrTopicNotFound is returned when a requested topic file does not exist.
	ErrTopicNotFound = errors.New("指定されたトピックが見つかりません")

	claudeProjectSeparator = regexp.MustCompile(`[^a-zA-Z0-9]`)
	topicPattern           = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

// Store manages repository-local shared memory under <repo-root>/.subcli/.
type Store struct {
	RepoRoot       string
	IsGitRepo      bool
	BaseDir        string
	MemoryDir      string
	MemoryFile     string
	ProjectionsDir string
}

// InitOptions controls Store.Init behavior.
type InitOptions struct {
	SkipGitignore bool
}

// InitResult reports what Store.Init created or updated.
type InitResult struct {
	RepoRoot         string
	MemoryFile       string
	CreatedMemory    bool
	UpdatedGitignore bool
}

// AddResult reports the outcome of Store.AddEntry.
type AddResult struct {
	TargetFile       string
	Topic            string
	Redacted         bool
	CreatedMemory    bool
	UpdatedGitignore bool
}

// ImportResult reports the outcome of Store.ImportMarkdownDir.
type ImportResult struct {
	SourceDir        string
	TargetFile       string
	ImportedFiles    []string
	Redacted         bool
	CreatedMemory    bool
	UpdatedGitignore bool
}

// FileInfo describes a canonical memory Markdown file.
type FileInfo struct {
	Name    string
	Topic   string
	Path    string
	Size    int64
	ModTime time.Time
	IsMain  bool
}

// StatusInfo summarizes the repository shared memory state.
type StatusInfo struct {
	RepoRoot            string
	IsGitRepo           bool
	MemoryDir           string
	MemoryFile          string
	Initialized         bool
	Gitignored          bool
	FileCount           int
	MainSizeBytes       int64
	ClaudeProjectionSet bool
	AgyProjectionSet    bool
}

// FindRepoRoot walks upward from startDir looking for a .git directory or file.
// If no Git marker is found, startDir itself is returned with isGit=false.
func FindRepoRoot(startDir string) (root string, isGit bool, err error) {
	if strings.TrimSpace(startDir) == "" {
		return "", false, errors.New("作業ディレクトリが空です")
	}
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return "", false, fmt.Errorf("作業ディレクトリの解決に失敗しました: %w", err)
	}
	cur := filepath.Clean(abs)
	for {
		gitMarker := filepath.Join(cur, ".git")
		if _, statErr := os.Lstat(gitMarker); statErr == nil {
			return cur, true, nil
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return "", false, fmt.Errorf("Git ディレクトリを確認できません: %w", statErr)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(abs), false, nil
		}
		cur = parent
	}
}

// Open resolves the repository root for startDir and returns a Store handle
// without modifying the filesystem.
func Open(startDir string) (*Store, error) {
	root, isGit, err := FindRepoRoot(startDir)
	if err != nil {
		return nil, err
	}
	return ForRoot(root, isGit), nil
}

// ForRoot constructs a Store rooted at repoRoot.
func ForRoot(repoRoot string, isGit bool) *Store {
	cleanRoot := filepath.Clean(repoRoot)
	baseDir := filepath.Join(cleanRoot, SubcliDirName)
	memDir := filepath.Join(baseDir, MemoryDirName)
	return &Store{
		RepoRoot:       cleanRoot,
		IsGitRepo:      isGit,
		BaseDir:        baseDir,
		MemoryDir:      memDir,
		MemoryFile:     filepath.Join(memDir, MainFileName),
		ProjectionsDir: filepath.Join(baseDir, ProjectionsDirName),
	}
}

// Initialized distinguishes absent memory from an unsafe or unreadable path.
func (s *Store) Initialized() (bool, error) {
	_, err := s.statFile(s.MemoryFile)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// HasTrackedMemory reports whether Git tracks any entry under .subcli.
// An ignored directory can still contain files tracked in an existing checkout.
func (s *Store) HasTrackedMemory() (bool, error) {
	if !s.IsGitRepo {
		return false, nil
	}
	cmd := exec.Command("git", "-C", s.RepoRoot, "-c", "core.fsmonitor=false", "ls-files", "--cached", "-z", "--", ":(icase,literal).subcli")
	// A caller's alternate index or repository must not mask the checkout's files.
	cmd.Env = []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf(".subcli の Git 管理状態を確認できません: %w", err)
	}
	return len(out) > 0, nil
}

// ProjectionDir returns the generated projection directory for a provider ID.
func (s *Store) ProjectionDir(providerID string) string {
	return filepath.Join(s.ProjectionsDir, providerID)
}

// Init creates <repo-root>/.subcli/memory/MEMORY.md (if absent), ensures
// .subcli/ is recorded in <repo-root>/.gitignore (unless SkipGitignore is set),
// and generates provider projections.
func (s *Store) Init(opts InitOptions) (InitResult, error) {
	dir, err := openDirBelow(s.RepoRoot, s.MemoryDir, true)
	if err != nil {
		return InitResult{}, err
	}
	dir.Close()

	res := InitResult{
		RepoRoot:   s.RepoRoot,
		MemoryFile: s.MemoryFile,
	}

	if _, err := s.statFile(s.MemoryFile); errors.Is(err, fs.ErrNotExist) {
		if err := s.writePrivateFile(s.MemoryFile, []byte(initialMainTemplate())); err != nil {
			return InitResult{}, fmt.Errorf("%s の作成に失敗しました: %w", s.MemoryFile, err)
		}
		res.CreatedMemory = true
	} else if err != nil {
		return InitResult{}, err
	}

	if s.IsGitRepo && !opts.SkipGitignore {
		updated, err := s.EnsureGitignore()
		if err != nil {
			return InitResult{}, err
		}
		res.UpdatedGitignore = updated
	}

	if err := s.Sync(); err != nil {
		return InitResult{}, err
	}
	return res, nil
}

// IsGitignored reports whether <repo-root>/.gitignore contains an entry covering .subcli.
func (s *Store) IsGitignored() (bool, error) {
	if !s.IsGitRepo {
		return false, nil
	}
	gitignorePath := filepath.Join(s.RepoRoot, ".gitignore")
	data, err := readFileBelow(s.RepoRoot, gitignorePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return gitignoreCoversSubcli(string(data)), nil
}

// EnsureGitignore appends .subcli/ to <repo-root>/.gitignore if not already ignored.
func (s *Store) EnsureGitignore() (bool, error) {
	if !s.IsGitRepo {
		return false, nil
	}
	gitignorePath := filepath.Join(s.RepoRoot, ".gitignore")
	var existing []byte
	data, err := readFileBelow(s.RepoRoot, gitignorePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf(".gitignore の読み込みに失敗しました: %w", err)
		}
	} else {
		existing = data
		if gitignoreCoversSubcli(string(existing)) {
			return false, nil
		}
	}

	var buf bytes.Buffer
	buf.Write(existing)
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		buf.WriteByte('\n')
	}
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString("# subcli local shared memory (not shared with team)\n")
	buf.WriteString(".subcli/\n")

	mode := os.FileMode(0o644)
	if info, statErr := s.statFile(gitignorePath); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := s.replaceFile(gitignorePath, buf.Bytes(), mode); err != nil {
		return false, fmt.Errorf(".gitignore の更新に失敗しました: %w", err)
	}
	return true, nil
}

func gitignoreCoversSubcli(content string) bool {
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch line {
		case ".subcli", ".subcli/", "/.subcli", "/.subcli/", ".subcli/*", "/.subcli/*", "**/.subcli", "**/.subcli/":
			return true
		}
	}
	return false
}

// ValidateTopic validates a topic name used as <topic>.md under .subcli/memory/.
func ValidateTopic(topic string) error {
	trimmed := strings.TrimSpace(topic)
	if trimmed == "" {
		return ErrInvalidTopic
	}
	if strings.EqualFold(trimmed, "MEMORY") || strings.EqualFold(trimmed, "MEMORY.md") {
		return ErrInvalidTopic
	}
	if strings.Contains(trimmed, "..") || strings.ContainsAny(trimmed, `/\`) {
		return ErrInvalidTopic
	}
	if !topicPattern.MatchString(trimmed) {
		return ErrInvalidTopic
	}
	return nil
}

// AddEntry appends a secret-redacted entry to MEMORY.md or <topic>.md.
// If the store is not initialized yet, it initializes it automatically.
func (s *Store) AddEntry(text string, topic string, now time.Time, opts InitOptions) (AddResult, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return AddResult{}, errors.New("記録するテキストが空です")
	}

	topic = strings.TrimSpace(topic)
	topic = strings.TrimSuffix(topic, ".md")
	if topic != "" {
		if err := ValidateTopic(topic); err != nil {
			return AddResult{}, err
		}
	}

	initRes, err := s.Init(opts)
	if err != nil {
		return AddResult{}, err
	}

	if now.IsZero() {
		now = time.Now()
	}

	redacted := logging.Redact(trimmed)
	wasRedacted := redacted != trimmed
	normalized := strings.ReplaceAll(redacted, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	entry := fmt.Sprintf("- [%s] %s", now.Format("2006-01-02"), lines[0])
	for _, extraLine := range lines[1:] {
		entry += "\n  " + extraLine
	}

	var targetFile string
	if topic == "" {
		targetFile = s.MemoryFile
		if err := s.appendMarkdownLine(targetFile, entry); err != nil {
			return AddResult{}, err
		}
	} else {
		targetFile = filepath.Join(s.MemoryDir, topic+".md")
		if _, statErr := s.statFile(targetFile); errors.Is(statErr, os.ErrNotExist) {
			header := fmt.Sprintf("# Topic: %s\n\n%s\n", topic, entry)
			if err := s.writePrivateFile(targetFile, []byte(header)); err != nil {
				return AddResult{}, fmt.Errorf("トピックファイルの作成に失敗しました: %w", err)
			}
		} else if statErr != nil {
			return AddResult{}, statErr
		} else {
			if err := s.appendMarkdownLine(targetFile, entry); err != nil {
				return AddResult{}, err
			}
		}
		if err := s.ensureTopicIndexLink(topic); err != nil {
			return AddResult{}, err
		}
	}

	if err := s.Sync(); err != nil {
		return AddResult{}, err
	}

	return AddResult{
		TargetFile:       targetFile,
		Topic:            topic,
		Redacted:         wasRedacted,
		CreatedMemory:    initRes.CreatedMemory,
		UpdatedGitignore: initRes.UpdatedGitignore,
	}, nil
}

// ReadMain returns the contents of MEMORY.md.
func (s *Store) ReadMain() (string, error) {
	data, err := readFileBelow(s.RepoRoot, s.MemoryFile)
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNotInitialized
	}
	if err != nil {
		return "", fmt.Errorf("%s の読み込みに失敗しました: %w", s.MemoryFile, err)
	}
	return string(data), nil
}

func (s *Store) checkInitialized() error {
	_, err := s.statFile(s.MemoryFile)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotInitialized
	}
	return err
}

// ReadTopic returns the contents of <topic>.md.
func (s *Store) ReadTopic(topic string) (string, error) {
	if err := s.checkInitialized(); err != nil {
		return "", err
	}
	topic = strings.TrimSuffix(strings.TrimSpace(topic), ".md")
	if err := ValidateTopic(topic); err != nil {
		return "", err
	}
	p := filepath.Join(s.MemoryDir, topic+".md")
	data, err := readFileBelow(s.RepoRoot, p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrTopicNotFound, topic)
		}
		return "", err
	}
	return string(data), nil
}

// List returns MEMORY.md (first) followed by sorted topic Markdown files.
func (s *Store) List() ([]FileInfo, error) {
	if err := s.checkInitialized(); err != nil {
		return nil, err
	}
	dir, err := openDirBelow(s.RepoRoot, s.MemoryDir, false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := fs.ReadDir(dir.FS(), ".")
	if err != nil {
		return nil, fmt.Errorf("メモリディレクトリの読み取りに失敗しました: %w", err)
	}

	var mainInfo *FileInfo
	var topics []FileInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		info, statErr := regularFile(dir, entry.Name())
		if statErr != nil {
			return nil, statErr
		}
		fi := FileInfo{
			Name:    entry.Name(),
			Path:    filepath.Join(s.MemoryDir, entry.Name()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if entry.Name() == MainFileName {
			fi.IsMain = true
			cp := fi
			mainInfo = &cp
		} else {
			fi.Topic = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			topics = append(topics, fi)
		}
	}

	sort.Slice(topics, func(i, j int) bool {
		return topics[i].Name < topics[j].Name
	})

	out := make([]FileInfo, 0, len(topics)+1)
	if mainInfo != nil {
		out = append(out, *mainInfo)
	}
	out = append(out, topics...)
	return out, nil
}

// RemoveTopic deletes <topic>.md, removes its index line from MEMORY.md, and
// refreshes projections.
func (s *Store) RemoveTopic(topic string) error {
	if err := s.checkInitialized(); err != nil {
		return err
	}
	topic = strings.TrimSuffix(strings.TrimSpace(topic), ".md")
	if err := ValidateTopic(topic); err != nil {
		return err
	}
	dir, err := openDirBelow(s.RepoRoot, s.MemoryDir, false)
	if err != nil {
		return err
	}
	defer dir.Close()
	name := topic + ".md"
	if _, err := regularFile(dir, name); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrTopicNotFound, topic)
		}
		return err
	}
	if err := s.removeTopicIndexLink(topic); err != nil {
		return fmt.Errorf("トピック %q の索引更新に失敗しました: %w", topic, err)
	}
	if err := dir.Remove(name); err != nil {
		return fmt.Errorf("トピック %q の削除に失敗しました: %w", topic, err)
	}
	return s.Sync()
}

// Sync regenerates provider projection files under .subcli/projections/ from
// the canonical .subcli/memory/ contents.
func (s *Store) Sync() error {
	mainContent, err := s.ReadMain()
	if err != nil {
		return err
	}
	files, err := s.List()
	if err != nil {
		return err
	}
	var topicPaths []string
	for _, f := range files {
		if !f.IsMain {
			topicPaths = append(topicPaths, f.Name)
		}
	}

	claudeDir := s.ProjectionDir("claude")
	claudeDoc := s.renderProjection("claude", mainContent, topicPaths, MaxProjectionBytes)
	if err := s.writePrivateFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(claudeDoc)); err != nil {
		return fmt.Errorf("claude 用メモリ投影の書き出しに失敗しました: %w", err)
	}

	agyDir := s.ProjectionDir("agy")
	agyDoc := s.renderProjection("agy", mainContent, topicPaths, MaxProjectionBytes)
	if err := s.writePrivateFile(filepath.Join(agyDir, "AGENTS.md"), []byte(agyDoc)); err != nil {
		return fmt.Errorf("agy 用メモリ投影の書き出しに失敗しました: %w", err)
	}

	return nil
}

// BuildProviderContext refreshes projections and returns the provider.MemoryContext
// to pass into a provider's MemoryAdapter.
func (s *Store) BuildProviderContext(providerID string) (provider.MemoryContext, error) {
	if err := s.Sync(); err != nil {
		return provider.MemoryContext{}, err
	}
	mainContent, err := s.ReadMain()
	if err != nil {
		return provider.MemoryContext{}, err
	}
	files, err := s.List()
	if err != nil {
		return provider.MemoryContext{}, err
	}
	var topicPaths []string
	for _, f := range files {
		if !f.IsMain {
			topicPaths = append(topicPaths, f.Name)
		}
	}

	return provider.MemoryContext{
		RepoRoot:      s.RepoRoot,
		MemoryDir:     s.MemoryDir,
		MemoryFile:    s.MemoryFile,
		ProjectionDir: s.ProjectionDir(providerID),
		InlinePrompt:  s.renderProjection(providerID, mainContent, topicPaths, MaxInlineArgBytes),
	}, nil
}

// Status returns a snapshot of the shared memory state for the repository.
func (s *Store) Status() (StatusInfo, error) {
	st := StatusInfo{
		RepoRoot:   s.RepoRoot,
		IsGitRepo:  s.IsGitRepo,
		MemoryDir:  s.MemoryDir,
		MemoryFile: s.MemoryFile,
	}
	ignored, err := s.IsGitignored()
	if err != nil {
		return st, err
	}
	st.Gitignored = ignored

	info, err := s.statFile(s.MemoryFile)
	if err == nil {
		st.Initialized = true
		st.MainSizeBytes = info.Size()
		files, err := s.List()
		if err != nil {
			return st, err
		}
		st.FileCount = len(files)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return st, err
	}
	for path, set := range map[string]*bool{
		filepath.Join(s.ProjectionDir("claude"), "CLAUDE.md"): &st.ClaudeProjectionSet,
		filepath.Join(s.ProjectionDir("agy"), "AGENTS.md"):    &st.AgyProjectionSet,
	} {
		if _, err := s.statFile(path); err == nil {
			*set = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return st, err
		}
	}
	return st, nil
}

// DefaultImportDir returns the default native Markdown memory directory for a
// supported provider ("claude" or "codex"). It never accesses credential files.
func DefaultImportDir(providerName, repoRoot, homeDir string, lookupEnv func(string) string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "claude":
		if strings.TrimSpace(homeDir) == "" {
			return "", errors.New("ホームディレクトリを取得できません")
		}
		encoded := EncodeClaudeProjectKey(repoRoot)
		return filepath.Join(homeDir, ".claude", "projects", encoded, "memory"), nil
	case "codex":
		if lookupEnv != nil {
			if codexHome := strings.TrimSpace(lookupEnv("CODEX_HOME")); codexHome != "" {
				return filepath.Join(codexHome, "memories"), nil
			}
		}
		if strings.TrimSpace(homeDir) == "" {
			return "", errors.New("ホームディレクトリを取得できません")
		}
		return filepath.Join(homeDir, ".codex", "memories"), nil
	default:
		return "", fmt.Errorf("未対応のインポート元プロバイダです: %q（claude または codex を指定してください）", providerName)
	}
}

// EncodeClaudeProjectKey converts an absolute repository path into the directory
// key format used under ~/.claude/projects/<key>/memory/.
func EncodeClaudeProjectKey(repoRoot string) string {
	clean := filepath.ToSlash(filepath.Clean(repoRoot))
	return claudeProjectSeparator.ReplaceAllString(clean, "-")
}

// ImportMarkdownDir reads .md files from sourceDir (read-only), redacts secrets,
// and writes a consolidated topic file imported-<provider>.md into .subcli/memory/.
func (s *Store) ImportMarkdownDir(sourceDir string, providerName string, now time.Time, opts InitOptions) (ImportResult, error) {
	prov := strings.ToLower(strings.TrimSpace(providerName))
	if prov != "claude" && prov != "codex" {
		return ImportResult{}, fmt.Errorf("未対応のインポート元プロバイダです: %q（claude または codex を指定してください）", providerName)
	}
	info, err := os.Lstat(sourceDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ImportResult{}, fmt.Errorf("インポート元メモリディレクトリが見つかりません: %s", sourceDir)
		}
		return ImportResult{}, err
	}
	if !info.IsDir() {
		return ImportResult{}, fmt.Errorf("インポート元パスはディレクトリではありません: %s", sourceDir)
	}

	var mdPaths []string
	err = filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			mdPaths = append(mdPaths, path)
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("インポート元ディレクトリの走査に失敗しました: %w", err)
	}
	sort.Strings(mdPaths)
	if len(mdPaths) == 0 {
		return ImportResult{}, fmt.Errorf("インポート可能な Markdown (.md) ファイルが %s に見つかりません", sourceDir)
	}

	initRes, err := s.Init(opts)
	if err != nil {
		return ImportResult{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}

	topic := "imported-" + prov
	targetFile := filepath.Join(s.MemoryDir, topic+".md")

	var buf strings.Builder
	fmt.Fprintf(&buf, "# Imported Memory (%s)\n\n", prov)
	fmt.Fprintf(&buf, "- Imported-At: %s\n", now.UTC().Format(time.RFC3339))
	fmt.Fprintf(&buf, "- Source-Dir: `%s`\n\n", sourceDir)

	var importedNames []string
	anyRedacted := false
	for _, p := range mdPaths {
		raw, readErr := readFileBelow(sourceDir, p)
		if readErr != nil {
			return ImportResult{}, fmt.Errorf("%s の読み込みに失敗しました: %w", p, readErr)
		}
		rel, _ := filepath.Rel(sourceDir, p)
		rel = filepath.ToSlash(rel)
		importedNames = append(importedNames, rel)

		rawStr := strings.TrimSpace(string(raw))
		redacted := logging.Redact(rawStr)
		if redacted != rawStr {
			anyRedacted = true
		}

		fmt.Fprintf(&buf, "## Source: %s\n\n", rel)
		buf.WriteString(redacted)
		buf.WriteString("\n\n")
	}

	if err := s.writePrivateFile(targetFile, []byte(buf.String())); err != nil {
		return ImportResult{}, fmt.Errorf("インポート先ファイルの書き出しに失敗しました: %w", err)
	}
	if err := s.ensureTopicIndexLink(topic); err != nil {
		return ImportResult{}, err
	}
	if err := s.Sync(); err != nil {
		return ImportResult{}, err
	}

	return ImportResult{
		SourceDir:        sourceDir,
		TargetFile:       targetFile,
		ImportedFiles:    importedNames,
		Redacted:         anyRedacted,
		CreatedMemory:    initRes.CreatedMemory,
		UpdatedGitignore: initRes.UpdatedGitignore,
	}, nil
}

func (s *Store) renderProjection(providerID, mainContent string, topicFiles []string, maxBytes int) string {
	var header strings.Builder
	header.WriteString("# subcli Shared Repository Memory\n\n")
	header.WriteString("This repository uses `subcli` local shared memory so knowledge persists across coding agents (Claude Code, Codex CLI, Antigravity CLI).\n")
	header.WriteString("Do NOT commit `.subcli/` (it is local-only and gitignored).\n\n")
	header.WriteString("## Memory Protocol\n")
	fmt.Fprintf(&header, "- Canonical memory file: `%s`\n", s.MemoryFile)
	fmt.Fprintf(&header, "- Shared memory directory: `%s`\n", s.MemoryDir)
	header.WriteString("- When the user asks you to remember a project decision, convention, command, or preference, update the canonical `MEMORY.md` (or a concise `<topic>.md` file in the shared memory directory and link it from `MEMORY.md`).\n")
	header.WriteString("- Keep entries concise, factual, and agent-agnostic. NEVER store API keys, passwords, OAuth tokens, or secrets.\n")
	if providerID != "" {
		fmt.Fprintf(&header, "- Active provider session: `%s`\n", providerID)
	}
	if len(topicFiles) > 0 {
		header.WriteString("\n## Available Topic Files (read on demand)\n")
		for _, name := range topicFiles {
			fmt.Fprintf(&header, "- `%s`\n", filepath.Join(s.MemoryDir, name))
		}
	}
	header.WriteString("\n## Current MEMORY.md\n\n")

	headerStr := header.String()
	body := strings.TrimSpace(logging.Redact(mainContent))
	if body == "" {
		return headerStr + "(empty)\n"
	}

	footerTruncated := fmt.Sprintf("\n\n<!-- truncated by subcli; read full file at %s -->\n", s.MemoryFile)
	if len(headerStr)+len(body)+1 <= maxBytes {
		return headerStr + body + "\n"
	}

	budget := maxBytes - len(headerStr) - len(footerTruncated)
	if budget <= 0 {
		return truncateUTF8Line(headerStr, maxBytes)
	}
	return headerStr + truncateUTF8Line(body, budget) + footerTruncated
}

func truncateUTF8Line(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= 0 {
		return ""
	}
	// Trim to valid UTF-8 boundary.
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	sub := s[:cut]
	// Prefer cutting at a newline boundary if reasonable.
	if nl := strings.LastIndexByte(sub, '\n'); nl > maxBytes/2 {
		return strings.TrimRight(sub[:nl], "\r\n")
	}
	return sub
}

func (s *Store) ensureTopicIndexLink(topic string) error {
	data, err := readFileBelow(s.RepoRoot, s.MemoryFile)
	if err != nil {
		return err
	}
	content := string(data)
	link := fmt.Sprintf("- [%s](./%s.md)", topic, topic)
	if strings.Contains(content, link) {
		return nil
	}

	const topicsHeading = "## Topics"
	if idx := strings.Index(content, topicsHeading); idx >= 0 {
		insertAt := idx + len(topicsHeading)
		if insertAt < len(content) && content[insertAt] == '\r' {
			insertAt++
		}
		if insertAt < len(content) && content[insertAt] == '\n' {
			insertAt++
		}
		updated := content[:insertAt] + link + "\n" + content[insertAt:]
		return s.writePrivateFile(s.MemoryFile, []byte(updated))
	}

	var buf strings.Builder
	buf.WriteString(content)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		buf.WriteByte('\n')
	}
	buf.WriteString("\n## Topics\n")
	buf.WriteString(link)
	buf.WriteByte('\n')
	return s.writePrivateFile(s.MemoryFile, []byte(buf.String()))
}

func (s *Store) removeTopicIndexLink(topic string) error {
	data, err := readFileBelow(s.RepoRoot, s.MemoryFile)
	if err != nil {
		return err
	}
	link := fmt.Sprintf("- [%s](./%s.md)", topic, topic)
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		if strings.TrimSpace(line) == link {
			changed = true
			continue
		}
		out = append(out, line)
	}
	if !changed {
		return nil
	}
	return s.writePrivateFile(s.MemoryFile, []byte(strings.Join(out, "\n")))
}

func (s *Store) appendMarkdownLine(path string, line string) error {
	data, err := readFileBelow(s.RepoRoot, path)
	if err != nil {
		return fmt.Errorf("%s の読み込みに失敗しました: %w", path, err)
	}
	var buf bytes.Buffer
	buf.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString(line)
	buf.WriteByte('\n')
	return s.writePrivateFile(path, buf.Bytes())
}

func initialMainTemplate() string {
	return `# Repository Shared Memory (subcli)

このファイルは subcli により管理されるリポジトリローカルの共有メモリです（.gitignore 対象・チーム非共有）。
Claude Code / Codex CLI / Antigravity CLI で共通参照・更新できます。

## Entries
`
}
