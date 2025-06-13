# Wisderly YouTube Script Generator

A Go-based application that automatically generates complete YouTube scripts using Google's Gemini API, with enhanced context awareness and modular architecture.

## Features

- **Modular Architecture**: Clean separation of concerns with dedicated services
- **Context-Aware Generation**: Maintains coherence across API calls without relying on Gemini's context system
- **Automatic Script Structure**: Generates outline, hook, introduction, 5 sections, and visual guidance
- **Enhanced Parsing**: Robust outline point extraction with multiple fallback strategies
- **Retry Logic**: Exponential backoff for API resilience
- **Progress Saving**: Incremental saves to prevent data loss
- **Configurable**: Easy to modify templates and generation parameters

## Architecture

### Core Components

```
main.go                 # Application entry point and user interaction
types.go               # Type definitions and data structures
gemini_service.go      # Gemini API client with retry logic
template_service.go    # Template management and prompt building
script_service.go      # Script generation orchestration
outline_parser.go      # Robust outline parsing with fallbacks
```

### Key Improvements

1. **Context Preservation**: Each API call includes context from previous content to maintain coherence
2. **Enhanced Parsing**: Multiple regex patterns and fallback strategies for outline extraction
3. **Retry Mechanism**: Exponential backoff for handling API rate limits and temporary failures
4. **Modular Templates**: Separate template files for different content types
5. **Session Management**: Comprehensive session state tracking and context building

## Installation

1. Clone the repository
2. Ensure Go 1.21+ is installed
3. Create required template files (see Template Files section)
4. Set up your Gemini API key

## Usage

```bash
# Run the application
go run .

# Follow the prompts:
# - Enter your video topic
# - Choose whether to generate visual guidance
# - The script will be automatically generated and saved
```

## Template Files

Create these template files in the same directory:

### `outline_template.txt`
```
Create a 5-point outline for a YouTube video about [TOPIC]...
```

### `script_template.txt`
```
Generate a 1000-1100 word section for a YouTube script...
```

### `hook_intro_template.txt`
```
Create an engaging hook and introduction for a YouTube video...
```

## Configuration

Modify these constants in `types.go` to adjust behavior:

```go
const (
    defaultSectionCount = 5
    defaultSleepBetweenSections = 15 * time.Second
    maxRetries = 3
)
```

## Context Management

The application maintains context between API calls through:

- **Previous Content**: Last portion of generated content for continuity
- **Key Themes**: Extracted from outline and maintained throughout
- **Tone & Style**: Consistent voice and approach
- **Content Summary**: High-level overview for context
- **Transition Phrases**: Natural flow between sections

## Error Handling

- **API Failures**: Exponential backoff retry mechanism
- **Template Issues**: Clear error messages and validation
- **Parsing Failures**: Multiple fallback strategies for outline extraction
- **File Operations**: Comprehensive error handling for saves

## Extensibility

Easy to extend with:
- Additional content types (conclusions, CTAs, etc.)
- Different AI providers (Claude, GPT, etc.)
- Custom template formats
- Alternative parsing strategies
- Enhanced context management

## File Structure

```
wisderly-script-generator/
├── main.go                 # Entry point
├── types.go               # Data structures
├── gemini_service.go      # API client
├── template_service.go    # Template management
├── script_service.go      # Generation orchestration
├── outline_parser.go      # Content parsing
├── go.mod                 # Go module
├── README.md             # Documentation
├── outline_template.txt   # Outline template
├── script_template.txt    # Script template
└── hook_intro_template.txt # Hook/intro template
```

## API Key Security

For production use:
1. Remove hardcoded API key from `main.go`
2. Use environment variable: `export GEMINI_API_KEY="your-key"`
3. Load with: `apiKey := os.Getenv("GEMINI_API_KEY")`

## Output

The generated script includes:
- Metadata header with topic and timestamp
- Complete outline (displayed in terminal only)
- Hook and introduction
- 5 detailed sections (1000-1100 words each)
- Visual guidance (if requested)
- Progress saves after each section

## Performance

- **Rate Limiting**: Built-in delays between API calls
- **Efficient Parsing**: Optimized regex patterns with fallbacks
- **Memory Management**: Streaming content building
- **Error Recovery**: Retry logic prevents total failures

## Contributing

1. Fork the repository
2. Create feature branch
3. Add tests for new functionality
4. Submit pull request

## License

MIT License - feel free to modify and distribute.