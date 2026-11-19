package frontmatter

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// Metadata represents the frontmatter data structure
// This can be expanded with additional fields in the future
type Metadata struct {
	Layout string `yaml:"layout,omitempty"`
	// Add additional fields here as needed
}

// split finds the frontmatter block at the start of content. The block opens
// with a "---" line and closes at the next line that starts with "---". Lines
// may end in "\n" or "\r\n". It returns the block's text without the
// delimiters, the content after the closing delimiter, and whether a block was
// found.
func split(content string) (block, rest string, ok bool) {
	var start int
	switch {
	case strings.HasPrefix(content, "---\n"):
		start = 4
	case strings.HasPrefix(content, "---\r\n"):
		start = 5
	default:
		return "", content, false
	}

	// Find the closing delimiter
	endDelimIndex := strings.Index(content[start:], "\n---")
	if endDelimIndex == -1 {
		return "", content, false
	}

	block = strings.TrimSuffix(content[start:start+endDelimIndex], "\r")
	return block, content[start+endDelimIndex+4:], true
}

// Parse extracts and parses frontmatter from markdown content
// Returns the parsed metadata and the content without frontmatter
func Parse(content string) (Metadata, string, bool) {
	var metadata Metadata

	fmContent, remainingContent, ok := split(content)
	if !ok {
		return metadata, content, false
	}

	// Parse the frontmatter as YAML
	err := yaml.Unmarshal([]byte(fmContent), &metadata)
	if err != nil {
		return metadata, content, false
	}

	// Remove any leading line breaks
	remainingContent = strings.TrimLeft(remainingContent, "\r\n")

	return metadata, remainingContent, true
}

// HasFrontmatter checks if content has frontmatter
func HasFrontmatter(content string) bool {
	_, _, ok := split(content)
	return ok
}

// Extract returns just the frontmatter as a string
func Extract(content string) string {
	block, _, _ := split(content)
	return block
}

// Add adds or updates frontmatter in content
func Add(content string, metadata Metadata) (string, error) {
	// Remove existing frontmatter if present
	_, contentWithoutFM, hasFM := Parse(content)
	if !hasFM {
		contentWithoutFM = content
	}

	// Marshal metadata to YAML
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(metadata); err != nil {
		return content, err
	}

	// Construct new content with frontmatter
	return "---\n" + buf.String() + "---\n\n" + contentWithoutFM, nil
}