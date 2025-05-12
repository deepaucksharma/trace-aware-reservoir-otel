package nrdot

import (
	"strings"
)

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Helper function to find the index of a substring
func indexOf(s, substr string) int {
	return strings.Index(s, substr)
}

// Helper function to add a line to a specific section of the distribution.yaml file
func addToSection(content, sectionStart, sectionEnd, lineToAdd string) string {
	startIdx := strings.Index(content, sectionStart)
	if startIdx < 0 {
		return content // Section start not found
	}
	
	// Move to end of the section header
	start := startIdx + len(sectionStart)
	
	// Find the end of the section
	endIdx := strings.Index(content[start:], sectionEnd)
	if endIdx < 0 {
		return content // Section end not found
	}
	
	end := start + endIdx
	
	// Extract the section
	section := content[start:end]
	
	// Check if the line already exists
	if strings.Contains(section, lineToAdd) {
		return content
	}
	
	// Add the line before the end of the section
	modified := content[:end] + "\n" + lineToAdd + content[end:]
	return modified
}