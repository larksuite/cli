package bot

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// CommandHandler defines a function that handles a bot command
type CommandHandler func(ctx context.Context, args []string, chatID string) (string, error)

// Router routes bot commands to their handlers
type Router struct {
	mu              sync.RWMutex
	commands        map[string]CommandHandler
	aliases         map[string]string
	whitelist       map[string]bool
	defaultHandler  CommandHandler
}

// RouterConfig configures a new Router
type RouterConfig struct {
	EnableWhitelist  bool
	WhitelistedCommands []string
	DefaultHandler  CommandHandler
}

// NewRouter creates a new command router
func NewRouter(config RouterConfig) *Router {
	r := &Router{
		commands:       make(map[string]CommandHandler),
		aliases:        make(map[string]string),
		whitelist:      make(map[string]bool),
		defaultHandler: config.DefaultHandler,
	}

	// Initialize whitelist if enabled
	if config.EnableWhitelist {
		for _, cmd := range config.WhitelistedCommands {
			r.whitelist[cmd] = true
		}
	}

	// Register built-in commands
	r.registerBuiltInCommands()

	return r
}

// RegisterCommand registers a new command handler
func (r *Router) RegisterCommand(command string, handler CommandHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return fmt.Errorf("command cannot be empty")
	}

	// Check whitelist if enabled
	if len(r.whitelist) > 0 && !r.whitelist[command] {
		return fmt.Errorf("command '%s' is not whitelisted", command)
	}

	r.commands[command] = handler
	return nil
}

// RegisterAlias registers a command alias
func (r *Router) RegisterAlias(alias string, target string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	alias = strings.ToLower(strings.TrimSpace(alias))
	target = strings.ToLower(strings.TrimSpace(target))

	if alias == "" || target == "" {
		return fmt.Errorf("alias and target cannot be empty")
	}

	if _, exists := r.commands[target]; !exists {
		return fmt.Errorf("target command '%s' does not exist", target)
	}

	r.aliases[alias] = target
	return nil
}

// Route routes a message to the appropriate command handler
func (r *Router) Route(ctx context.Context, message string, chatID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	message = strings.TrimSpace(message)
	if message == "" {
		return "", nil
	}

	// Check if message is a command
	if !strings.HasPrefix(message, "/") {
		// Not a command, use default handler
		if r.defaultHandler != nil {
			return r.defaultHandler(ctx, []string{message}, chatID)
		}
		return "", fmt.Errorf("no default handler registered")
	}

	// Parse command and arguments
	parts := strings.Fields(message)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	command := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}

	// Resolve alias
	if target, isAlias := r.aliases[command]; isAlias {
		command = target
	}

	// Find handler
	handler, exists := r.commands[command]
	if !exists {
		// Unknown command, use default handler
		if r.defaultHandler != nil {
			return r.defaultHandler(ctx, []string{message}, chatID)
		}
		return "", fmt.Errorf("unknown command: /%s", command)
	}

	// Execute handler
	return handler(ctx, args, chatID)
}

// ListCommands returns all registered commands
func (r *Router) ListCommands() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var commands []string
	for cmd := range r.commands {
		commands = append(commands, cmd)
	}
	return commands
}

// registerBuiltInCommands registers default bot commands
func (r *Router) registerBuiltInCommands() {
	// /status - Show bot status
	r.RegisterCommand("status", func(ctx context.Context, args []string, chatID string) (string, error) {
		return "Bot is running. Active sessions: see stats for details.", nil
	})

	// /help - Show available commands
	r.RegisterCommand("help", func(ctx context.Context, args []string, chatID string) (string, error) {
		r.mu.RLock()
		defer r.mu.RUnlock()

		help := "Available commands:\n"
		for cmd := range r.commands {
			help += fmt.Sprintf("  /%s\n", cmd)
		}
		return help, nil
	})

	// /clear - Clear current session
	r.RegisterCommand("clear", func(ctx context.Context, args []string, chatID string) (string, error) {
		// This will be implemented with session manager
		return "Session cleared. Starting a new conversation.", nil
	})
}

// MessagePattern represents a regex-based message routing pattern
type MessagePattern struct {
	Regex    *regexp.Regexp
	Handler  CommandHandler
	Priority int // Higher priority = checked first
}

// PatternRouter routes messages based on regex patterns
type PatternRouter struct {
	mu        sync.RWMutex
	patterns  []MessagePattern
	fallback  CommandHandler
}

// NewPatternRouter creates a new pattern-based router
func NewPatternRouter() *PatternRouter {
	return &PatternRouter{
		patterns: make([]MessagePattern, 0),
	}
}

// AddPattern adds a routing pattern
func (pr *PatternRouter) AddPattern(pattern string, handler CommandHandler, priority int) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	pr.patterns = append(pr.patterns, MessagePattern{
		Regex:    regex,
		Handler:  handler,
		Priority: priority,
	})

	// Sort by priority (highest first)
	pr.sortPatterns()

	return nil
}

// SetFallback sets the fallback handler for unmatched messages
func (pr *PatternRouter) SetFallback(handler CommandHandler) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.fallback = handler
}

// Route routes a message using pattern matching
func (pr *PatternRouter) Route(ctx context.Context, message string, chatID string) (string, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	for _, pattern := range pr.patterns {
		if pattern.Regex.MatchString(message) {
			return pattern.Handler(ctx, []string{message}, chatID)
		}
	}

	// Use fallback if no pattern matched
	if pr.fallback != nil {
		return pr.fallback(ctx, []string{message}, chatID)
	}

	return "", fmt.Errorf("no matching pattern for message")
}

// sortPatterns sorts patterns by priority (highest first)
func (pr *PatternRouter) sortPatterns() {
	// Simple bubble sort (pattern count is typically small)
	n := len(pr.patterns)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if pr.patterns[j].Priority < pr.patterns[j+1].Priority {
				pr.patterns[j], pr.patterns[j+1] = pr.patterns[j+1], pr.patterns[j]
			}
		}
	}
}
