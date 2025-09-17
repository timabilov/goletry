package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"google.golang.org/genai"
)

// LLMModelName is the GenAI backend to use for the client.
type LLMModelName int32

const (
	// BackendUnspecified causes the backend determined automatically. If the
	// GOOGLE_GENAI_USE_VERTEXAI environment variable is set to "1" or "true", then
	// the backend is BackendVertexAI. Otherwise, if GOOGLE_GENAI_USE_VERTEXAI
	// is unset or set to any other value, then BackendGeminiAPI is used.  Explicitly
	// setting the backend in ClientConfig overrides the environment variable.
	Pro25 LLMModelName = iota
	// BackendGeminiAPI is the Gemini API backend.
	Flash25
	FlashLite25
	Flash20
)

// The Stringer interface for Backend.
func (t LLMModelName) String() string {
	switch t {
	case Pro25:
		return "gemini-2.5-pro"
	case Flash25:
		return "gemini-2.5-flash"
	case FlashLite25:
		return "gemini-2.5-flash-lite-preview-06-17"
	case Flash20:
		return "gemini-2.0-flash"
	default:
		return "gemini-2.0-flash"
	}
}

func floatPointer(f float32) *float32 {
	return &f
}

type LLMResponse struct {
	Response           string `json:"response"`
	InputTokenCount    int32  `json:"input_token_count"`
	Thoughts           string `json:"thoughts"`
	ThoughtsTokenCount int32  `json:"thoughts_token_count"`
	OutputTokenCount   int32  `json:"output_token_count"`
	TotalTokenCount    int32  `json:"total_token_count"`
	IsTest             bool   `json:"is_test"`
	// to add
}

type LLMNoteProcessor interface {
	Transcribe(filePaths []string, modelName LLMModelName, languageCode string) (*LLMResponse, error)
	ImageOrPdfParse(filePaths []string, modelName LLMModelName) (*LLMResponse, error)
	DocumentsParse(filePaths []string, userTranscript string, modelName LLMModelName, languageCode string) (*LLMResponse, error)
	TextParse(text string, modelName LLMModelName) (*LLMResponse, error)
	ExamParse(text *string, filePaths []string, modelName LLMModelName) (*LLMResponse, error)
	GenerateQuizAndFlashCards(content string, isSourceTest bool, modelName LLMModelName, languageCode string) (*LLMResponse, error)
}

type QuizObject struct {
	Question    string   `json:"question"`
	Answer      uint     `json:"answer"`
	Options     []string `json:"options"`
	Explanation string   `json:"explanation"`
}

type FlashCard struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}
type NoteQuizResponse struct {
	FailedToProcessLLMNoteResponse string       `gorm:"type:text" json:"-"`
	EasyQuestions                  []QuizObject `json:"easy_questions"`
	MediumQuestions                []QuizObject `json:"medium_questions"`
	HardQuestions                  []QuizObject `json:"hard_questions"`
	BonusQuestions                 []QuizObject `json:"bonus_questions"`
	FlashCards                     []FlashCard  `json:"flashcards"`
}

type NoteTranscribeResponse struct {
	Name          string `json:"name"`
	MD_Summary    string `json:"md_summary"`
	Transcription string `json:"transcription"`
	Language      string `json:"language"`
}

type GoogleLLMNoteProcessor struct{}

func Int64Pointer(i int64) *int64 {
	return &i
}

func Int32Pointer(i int32) *int32 {
	return &i
}

type ResponseWithThoughts struct {
	Thoughts string `json:"thoughts"`
	Text     string `json:"text"`
}

func tryUploadGoogleStorage(ctx context.Context, client *genai.Client, filePath string, newName *string) (*genai.File, error) {
	var genFile *genai.File
	var err error
	maxUploadTimes := 3
	for i := range maxUploadTimes {
		config := &genai.UploadFileConfig{}
		if newName != nil {
			config = &genai.UploadFileConfig{
				Name: *newName,
			}
		}
		genFile, err = client.Files.UploadFromPath(ctx, filePath, config)
		if err == nil {

			fmt.Println("File uploaded successfully:", filePath, "Attempt:", i+1)
			return genFile, nil
		}
		fmt.Printf("Error uploading file %s, attempt %d: %v\n", filePath, i+1, err)
	}
	return nil, fmt.Errorf("failed to upload file to google storage /after %d attempts: %s", maxUploadTimes, filePath)
}
func GetFirstCandidateTextWithThoughts(result *genai.GenerateContentResponse) (*ResponseWithThoughts, error) {
	var thinkingContent string
	for _, c := range result.Candidates {
		// fmt.Println("Candidate:", i, c.Content.Parts[0].Text, c.Content.Parts[0].Thought)
		fmt.Println("Finish reason: ", c.FinishReason, " Finish message: ", c.FinishMessage)
		if len(c.SafetyRatings) > 0 {
			for _, rating := range c.SafetyRatings {
				fmt.Println("[Safety] rating:", rating.Category, "Score:", rating.Probability, "Severity score:", rating.SeverityScore, " Blocked:", rating.Blocked)
				if rating.Blocked {
					return nil, fmt.Errorf("content violation: Couldn't analyze the note, because it contains %s,", rating.Category)
				}
			}
		}
		for _, part := range c.Content.Parts {
			if part.Thought && part.Text != "" {
				if result.UsageMetadata != nil && result.UsageMetadata.ThoughtsTokenCount > 25000 {
					fmt.Println("Warning: Thought content is too long:", result.UsageMetadata.ThoughtsTokenCount, part.Text)
				}
				thinkingContent = part.Text
				continue
			}

		}
	}
	text := result.Text()
	if len(text) == 0 {
		return nil, fmt.Errorf("no content response found in response")
	}
	return &ResponseWithThoughts{
		Thoughts: thinkingContent,
		Text:     result.Text(),
	}, nil
}

func (GoogleLLMNoteProcessor) Transcribe(filePaths []string, modelName LLMModelName, languageCode string) (*LLMResponse, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GOOGLE_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})

	var genFiles []*genai.File
	// Upload each file and get the URI
	for _, filePath := range filePaths {
		// fmt.Println("File path for image parse:", filePath)
		// try to upload couple of times if err, default 3
		genFile, err := tryUploadGoogleStorage(ctx, client, filePath, nil)
		if err != nil {
			fmt.Println("Error uploading file:", filePath, err)
			return nil, fmt.Errorf("error uploading file %s: %v", filePath, err)
		}
		genFiles = append(genFiles, genFile)
	}

	var parts []*genai.Part
	// generate pars from for each file then merge it with text
	for i, genFile := range genFiles {
		fmt.Println("File path for image parse:", i, " ", genFile.URI, genFile.MIMEType)
		parts = append(parts, &genai.Part{
			FileData: &genai.FileData{
				FileURI:  genFile.URI,
				MIMEType: genFile.MIMEType,
			},
		})
	}

	if err != nil {
		log.Fatal(err)
	}
	//Please use ONLY the following JSON schema for the response:
	//Note = {'name': string, 'md_summary': string, 'transcription': string, 'quiz_json': Array<{'question': string, 'answer': string, 'options': Array<string>}>}
	//Return: Note},
	// result, err := client.Models.GenerateContent(ctx, "gemini-2.5-pro-preview-03-25", []*genai.Content{{Parts: parts}}, nil)
	languagePrompt := `Detect the language of audio and stick to it.  Do not assume the language based on the first word or phrase, especially if it appears unusual or out of context. Analyze the first six to seven sentences to accurately detect the primary language of the text. Do not analyze whole audio to identify primary language, couple sentences are enough.`

	if languageCode != "auto" {
		languagePrompt = `Transctipion language should be in "` + languageCode + ` code". (ISO 639-1 code). The main transcription text language should be "` + languageCode + `".`
	}

	languagePrefix := `original identified language`
	if languageCode != "auto" {
		languagePrefix = `"` + languageCode + ` code language"`
	}
	result, err := client.Models.GenerateContent(ctx, modelName.String(), []*genai.Content{{Parts: parts}}, &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		CandidateCount:   1,
		ThinkingConfig: &genai.ThinkingConfig{
			IncludeThoughts: true,
			ThinkingBudget:  Int32Pointer(3000),
		},
		// because its youtube it can have more..
		MaxOutputTokens: 50000,
		Temperature:     floatPointer(0.8),
		// TopK:            floatPointer(0.5),
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: `You are expert at transcribing the whole audio with the proper grammar.` + languagePrompt + `Follow the below instructions.
1. Your response must be valid JSON, and nothing else. Return only these fields: transcription, name, md_summary, language. Ensure all string values are properly JSON-escaped. For big enough transcription ensure that you donot lose JSON formatting at the end.
JSON Fields:
name - A concise subject name in the ` + languagePrefix + ` only
md_summary - A summary in the ` + languagePrefix + ` using Markdown language. Begin with a brief introduction, followed by key points, events, and facts. Include a table for related key facts. Use only these Markdown directives as needed: bold, table elements, heading1, heading2, heading3, line breaks, new lines, emphasis, italic, blockquotes, lists (non-nested), code blocks, inline code. Do not use LaTeX, HTML, or nested lists.
transcription - A complete, accurate transcription of whole audio into the ` + languagePrefix + ` Markdown text, using correct grammar, punctuation, and formatting. Transcribe it exactly and with special attention to nouns. Do not include timestamps. Focus on grammatical correctness and use sentence breaks to represent natural pauses. Add commas and quotes where needed for clarity. Use complete sentences and a readable layout with paragraphs and line breaks. Do not alter sentence structure. Use only plain symbols and Markdown language for formulas or similar content. Do not use LaTeX, HTML, or illustrate graphics with symbol art or Markdown. Transcribe all audio speech. After big transcription DO NOT forget that you are still returning JSON object and after finishing make sure to  STICK to JSON formatting rule by closing it with double qoutes and either with bracket or comma if there is additional field needed.
language - The primary content ` + languagePrefix + ` in ISO 639-1 format (2-letter code, e.g., 'en' for English).

Example:
{
  "name": "Example Subject",
  "md_summary": "## Summary\\nThis is a summary of the content.\\n\\n### Key Points\\n- Point 1\\n- Point 2\\n\\n| Fact | Value |\\n|------|-------|\\n| Example | 123 |",
  "transcription": "This is the transcription of the audio content.\\nIt includes all spoken words and phrases, formatted correctly with punctuation and grammar. Although it might be very long transcription, we donot forget that this is JSON and we need close it with double quotes",
  "language": "en"
}
`},
			},
		},
		ResponseSchema: &genai.Schema{
			Type: "object",

			// The schema for the AI response.
			Properties: map[string]*genai.Schema{
				"name": {
					Type:        "string",
					Description: "A concise subject name in the original identified content language only",
				},
				"md_summary": {
					Type:        "string",
					Description: "A summary in the original identified language using Markdown language. Begin with a brief introduction, followed by key points, events, and facts. Include a table for related key facts. Use only these Markdown directives as needed: bold, table elements, heading1, heading2, heading3, line breaks, new lines, emphasis, italic, blockquotes, lists (non-nested), code blocks, inline code. Do not use LaTeX, HTML, or nested lists.",
				},
				"transcription": {
					Type:        "string",
					Description: "A complete, accurate transcription of whole audio into the identified language Markdown text, using correct grammar, punctuation, and formatting. Transcribe it exactly and with special attention to nouns. Do not include timestamps. Focus on grammatical correctness and use sentence breaks to represent natural pauses. Add commas and quotes where needed for clarity. Use complete sentences and a readable layout with paragraphs and line breaks. Do not alter sentence structure. Use only plain symbols and Markdown language for formulas or similar content. Do not use LaTeX, HTML, or illustrate graphics with symbol art or Markdown. Transcribe all audio speech to the end",
				},
				"language": {
					Type:        "string",
					Description: "The primary content language in ISO 639-1 format (2-letter code, e.g., 'en' for English)",
				},
			},
			Required: []string{"name", "md_summary", "transcription", "language"},
		},
	})
	// client.Models.Cou

	if err != nil {
		fmt.Println("Error in GenerateContent:", err)
		return nil, fmt.Errorf("%v", err)
	}
	inputTokenCount := result.UsageMetadata.PromptTokenCount
	thoughtsTokenCount := result.UsageMetadata.ThoughtsTokenCount
	outpuTokenCount := result.UsageMetadata.CandidatesTokenCount
	totalTokenCount := result.UsageMetadata.TotalTokenCount
	fmt.Println("Input token count:", inputTokenCount)
	fmt.Println("Output token count:", outpuTokenCount)
	fmt.Println("Thoughts token count:", thoughtsTokenCount)
	fmt.Println("Total token count:", totalTokenCount)
	llmResponseText, err := GetFirstCandidateTextWithThoughts(result)
	if err != nil {
		fmt.Println("Error getting first candidate text: ", err)

		fmt.Println(result.Candidates)
		if result.PromptFeedback != nil {

			fmt.Println(result.PromptFeedback.BlockReason)
			fmt.Println(result.PromptFeedback.BlockReasonMessage)
			fmt.Println(result.PromptFeedback.SafetyRatings)
			return nil, fmt.Errorf("content violation: %s ", result.PromptFeedback.BlockReasonMessage)
		}
		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	}
	return &LLMResponse{
		Response:           llmResponseText.Text,
		Thoughts:           llmResponseText.Thoughts,
		InputTokenCount:    inputTokenCount,
		ThoughtsTokenCount: thoughtsTokenCount,
		OutputTokenCount:   outpuTokenCount,
		TotalTokenCount:    totalTokenCount,
		IsTest:             false,
	}, nil

}

func (GoogleLLMNoteProcessor) ImageOrPdfParse(filePaths []string, modelName LLMModelName) (*LLMResponse, error) {
	ctx := context.Background()
	// fileName
	fmt.Println("File path for image parse:", filePaths)
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		//GOOGLE_API_KEY env
		APIKey:  os.Getenv("GOOGLE_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	var genFiles []*genai.File
	// Upload each file and get the URI
	for _, filePath := range filePaths {
		// fmt.Println("File path for image parse:", filePath)
		genFile, err := client.Files.UploadFromPath(ctx, filePath, &genai.UploadFileConfig{})
		if err != nil {
			return nil, fmt.Errorf("Error uploading GEN AI image file: %s %v", filePath, err)
		}
		genFiles = append(genFiles, genFile)
	}

	var parts []*genai.Part
	// generate pars from for each file then merge it with text
	for i, genFile := range genFiles {
		fmt.Println("File path for image parse:", i, " ", genFile.URI, genFile.MIMEType)
		parts = append(parts, &genai.Part{
			FileData: &genai.FileData{
				FileURI:  genFile.URI,
				MIMEType: genFile.MIMEType,
			},
		})
	}
	// donot use html elements such as sub sup
	// Analyze the image text and pay special attention to precision. We need to return:
	// 1. Proper complete image text with original transcript language (with accompanying illustration explanation in braces if exists). Ensure transcript precision and technical context is consistent and correct and also format it to have correct commas, quotes, parentheses and generally proper readable text, return only transcription, no timestamps. Additionally, For each highly unclear word provide placeholder of possible options and their possibility threshold in form of this example: "  [gone:0.7][done:0.5] " meaning that we are not sure about this word/phrase.
	// 2. Concise subject name on original text content language only.
	// 3. Markdown Format Summary in original content language: Start with brief introduction then specify key points, events, facts. You can generate a table for similar key facts.
	// If image is empty or not understandable, please return only "Unknown note" for the name and keep other fields empty.
	// 4. Quiz Object: Generate quiz with 10 questions and answers based on the image content and the language of the content. The quiz should be in JSON format. Each question should have a question, answer, and options. The 'options' should be in an array of strings containing four options. The 'answer' should be the correct answer from the options array.
	// Return the result in JSON format.
	parts = append(parts, &genai.Part{
		Text: `Analyze the image or pdf text and pay special attention to precision. We need to return:
1. Return proper complete image text using Markdown but with correct grammar, punctuation, and formatting in the original language, adding commas and quotes where needed for clarity. Donot use latex at all. For illustrations briefly mention them in original content language and in a separate paragraph. Ensure complete sentences and a readable layout. For words or phrases clearly out of context, replace with a placeholder and provide three options: the original word/phrase (first), and two alternatives, with possibility thresholds in the format original:probability][alternative1:probability][alternative2:probability], e.g., [original:0.4][gone:0.7][done:0.5]. Apply placeholders only for definitively out-of-context words/phrases.
2. Concise subject name on original text content language only.
3. Summarize in the original content language using Markdown. Donot use latex at all. Begin with a brief introduction, followed by key points, events, and facts. Use a table for related key facts. Avoid nested lists. Only these markdown directives allowed if needed: bold, table elements, heading1, heading2, heading3, line breaks, new lines, emphasis, italic, blockquotes, any lists (not nested), code blocks, inline code
If image is empty or not understandable, please return only "Unknown note" for the name and keep other fields empty.
4. Content language in ISO 639 format (2 symbols).
`,
	})
	if err != nil {
		log.Fatal(err)
	}
	//Please use ONLY the following JSON schema for the response:
	//Note = {'name': string, 'md_summary': string, 'transcription': string, 'quiz_json': Array<{'question': string, 'answer': string, 'options': Array<string>}>}
	//Return: Note},
	// result, err := client.Models.GenerateContent(ctx, "gemini-2.5-pro-preview-03-25", []*genai.Content{{Parts: parts}}, nil)
	result, err := client.Models.GenerateContent(ctx, modelName.String(), []*genai.Content{{Parts: parts}}, &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		ResponseSchema: &genai.Schema{
			Type: "object",
			// The schema for the AI response.
			Properties: map[string]*genai.Schema{
				"name": {
					Type: "string",
				},
				"md_summary": {
					Type: "string",
				},
				"transcription": {
					Type: "string",
				},
				"language": {
					Type: "string",
				},
			},
			Required: []string{"name", "md_summary", "transcription", "language"},
		},
		ThinkingConfig: &genai.ThinkingConfig{
			IncludeThoughts: true,
			// ThinkingBudget:  Int32Pointer()
		},
	})

	// client.Models.Cou

	if err != nil {
		fmt.Println("Error in GenerateContent:", err)
		return nil, fmt.Errorf("%v", err)
	}
	var inputTokenCount int32
	var thoughtsTokenCount int32
	var outputTokenCount int32
	var totalTokenCount int32
	if result.UsageMetadata != nil {
		inputTokenCount = result.UsageMetadata.PromptTokenCount
		thoughtsTokenCount = result.UsageMetadata.ThoughtsTokenCount
		outputTokenCount = result.UsageMetadata.CandidatesTokenCount
		totalTokenCount = result.UsageMetadata.TotalTokenCount
	} else {
		fmt.Println("UsageMetadata is nil!")
	}
	llmResponseText, err := GetFirstCandidateTextWithThoughts(result)
	if err != nil {
		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	}
	return &LLMResponse{
		Response:           llmResponseText.Text,
		Thoughts:           llmResponseText.Thoughts,
		InputTokenCount:    inputTokenCount,
		ThoughtsTokenCount: thoughtsTokenCount,
		OutputTokenCount:   outputTokenCount,
		TotalTokenCount:    totalTokenCount,
		IsTest:             false,
	}, nil

}

var dashAlphaRule = regexp.MustCompile(`[^a-zA-Z0-9-]`)

func (GoogleLLMNoteProcessor) DocumentsParse(filePaths []string, userTranscript string, modelName LLMModelName, languageCode string) (*LLMResponse, error) {
	ctx := context.Background()
	// fileName
	fmt.Println("File path for image parse:", filePaths)
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		//GOOGLE_API_KEY env
		APIKey:  os.Getenv("GOOGLE_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	var genFiles []*genai.File
	// Upload each file and get the URI
	for _, filePath := range filePaths {
		// fmt.Println("File path for image parse:", filePath)
		fileName := filepath.Base(filePath)
		sanitizedFileName := dashAlphaRule.ReplaceAllString(strings.ReplaceAll(fileName, ".", "-"), "")
		genFile, err := tryUploadGoogleStorage(ctx, client, filePath, &sanitizedFileName)
		if err != nil {
			fmt.Println("Error uploading file:", filePath, err)
			return nil, fmt.Errorf("error uploading file to google storage %s: %v", filePath, err)
		}
		genFiles = append(genFiles, genFile)
	}

	var parts []*genai.Part
	// generate pars from for each file then merge it with text
	for i, genFile := range genFiles {
		fmt.Println("File path for image parse:", i, " ", genFile.URI, genFile.MIMEType)
		parts = append(parts, &genai.Part{
			FileData: &genai.FileData{
				FileURI:  genFile.URI,
				MIMEType: genFile.MIMEType,
			},
		})
	}
	// donot use html elements such as sub sup
	// Analyze the image text and pay special attention to precision. We need to return:
	// 1. Proper complete image text with original transcript language (with accompanying illustration explanation in braces if exists). Ensure transcript precision and technical context is consistent and correct and also format it to have correct commas, quotes, parentheses and generally proper readable text, return only transcription, no timestamps. Additionally, For each highly unclear word provide placeholder of possible options and their possibility threshold in form of this example: "  [gone:0.7][done:0.5] " meaning that we are not sure about this word/phrase.
	// 2. Concise subject name on original text content language only.
	// 3. Markdown Format Summary in original content language: Start with brief introduction then specify key points, events, facts. You can generate a table for similar key facts.
	// If image is empty or not understandable, please return only "Unknown note" for the name and keep other fields empty.
	// 4. Quiz Object: Generate quiz with 10 questions and answers based on the image content and the language of the content. The quiz should be in JSON format. Each question should have a question, answer, and options. The 'options' should be in an array of strings containing four options. The 'answer' should be the correct answer from the options array.
	// Return the result in JSON format.
	if len(userTranscript) > 0 {
		parts = append(parts, &genai.Part{
			Text: userTranscript,
		})
	}
	if err != nil {
		log.Fatal(err)
	}
	//Please use ONLY the following JSON schema for the response:
	//Note = {'name': string, 'md_summary': string, 'transcription': string, 'quiz_json': Array<{'question': string, 'answer': string, 'options': Array<string>}>}
	//Return: Note},
	// result, err := client.Models.GenerateContent(ctx, "gemini-2.5-pro-preview-03-25", []*genai.Content{{Parts: parts}}, nil)

	languagePrompt := `Detect the language of content and stick to it. Do not assume the language based on the first word or phrase, especially if it appears unusual or out of context. Analyze the first six to seven sentences of each file to accurately detect the primary language of the text.`

	if languageCode != "auto" {
		languagePrompt = `Transctipion language should be in "` + languageCode + ` code". (ISO 639-1 code). The main transcription text language should be "` + languageCode + `".`
	}
	languagePrefix := `identified primary language`
	if languageCode != "auto" {
		languagePrefix = `"` + languageCode + ` code language"`
	}
	result, err := client.Models.GenerateContent(ctx, modelName.String(), []*genai.Content{{Parts: parts}}, &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		CandidateCount:   1,
		MaxOutputTokens:  50000,
		Temperature:      floatPointer(0.8),
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: `You are expert at transcribing and analyzing images, PDFs, audio files, and text content with the proper grammar. ` + languagePrompt + ` Follow the below instructions. Do not deviate from these requirements.  Return the response in JSON format with the specified fields.

1. For audio content, transcribe it exactly in the ` + languagePrefix + `, ensuring absolute precision in nouns. Do not include timestamps. Use complete sentences with correct grammar and punctuation. For multiple audio files, transcribe these files preserving the chronological order as much as possible only based on the timestamp in their filenames or part numbers.
2. For images and PDFs, transcribe the text while preserving the readable layout and pages order as closely as possible. In case of provided exam/test questions - do not solve them no matter what, only transcribe them. For any illustrations, include a brief description in the  ` + languagePrefix + ` within the transcription.
3. Return the following JSON fields and make sure "\n" is escaped properly on every fields value as "\\n":
   - **transcription**: A complete, accurate transcription of all material text in Markdown language, using correct grammar, punctuation, and formatting in the ` + languagePrefix + `. Add commas and quotes where needed for clarity. Focus on grammatical correctness and use sentence breaks to represent natural pauses. Use complete sentences and a readable layout with paragraphs and line breaks. Do not alter sentence structure. Use only plain symbols and Markdown for formulas or similar content. Do not use LaTeX, HTML, or illustrate graphics with symbol art or Markdown.
   - **name**: A concise subject name in the ` + languagePrefix + ` only.
   - **md_summary**: A summary in the ` + languagePrefix + ` using Markdown. Begin with a brief introduction, followed by key points, events, and facts. Include a table for related key facts. Use only these Markdown directives if needed: bold, table elements, heading1, heading2, heading3, line breaks, new lines, emphasis, italic, blockquotes, lists (non-nested), code blocks, inline code. Do not use LaTeX, HTML, or nested lists.
   - **language**: The i ` + languagePrefix + ` in ISO 639-1 format (2-letter code, e.g., "en" for English).

Ensure all instructions are followed precisely, with no omissions or alterations. The response must be in JSON format, and all fields must be populated correctly based on the provided material.`},
			},
		},
		ResponseSchema: &genai.Schema{
			Type: "object",
			// The schema for the AI response.
			Properties: map[string]*genai.Schema{
				"name": {
					Type: "string",
				},
				"md_summary": {
					Type: "string",
				},
				"transcription": {
					Type: "string",
				},
				"language": {
					Type: "string",
				},
			},
			Required: []string{"name", "md_summary", "transcription", "language"},
		},
		ThinkingConfig: &genai.ThinkingConfig{
			IncludeThoughts: true,
			// ThinkingBudget:  Int32Pointer()
		},
	})

	// client.Models.Cou

	if err != nil {
		fmt.Println("Error in GenerateContent:", err)
		return nil, fmt.Errorf("%v", err)
	}
	var inputTokenCount int32
	var thoughtsTokenCount int32
	var outputTokenCount int32
	var totalTokenCount int32
	if result.UsageMetadata != nil {
		inputTokenCount = result.UsageMetadata.PromptTokenCount
		thoughtsTokenCount = result.UsageMetadata.ThoughtsTokenCount
		outputTokenCount = result.UsageMetadata.CandidatesTokenCount
		totalTokenCount = result.UsageMetadata.TotalTokenCount
		fmt.Println("Input token count:", inputTokenCount)
		fmt.Println("Output token count:", outputTokenCount)
		fmt.Println("Thoughts token count:", thoughtsTokenCount)
		fmt.Println("Total token count:", totalTokenCount)
	} else {
		fmt.Println("UsageMetadata is nil!")
	}
	llmResponseText, err := GetFirstCandidateTextWithThoughts(result)
	if err != nil {
		fmt.Println("Error getting first candidate text: ", err)
		fmt.Println(result.Candidates)
		if result.PromptFeedback != nil {

			fmt.Println(result.PromptFeedback.BlockReason)
			fmt.Println(result.PromptFeedback.BlockReasonMessage)
			fmt.Println(result.PromptFeedback.SafetyRatings)
			return nil, fmt.Errorf("content violation: %s ", result.PromptFeedback.BlockReasonMessage)
		}
		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	}
	return &LLMResponse{
		Response:           llmResponseText.Text,
		Thoughts:           llmResponseText.Thoughts,
		InputTokenCount:    inputTokenCount,
		ThoughtsTokenCount: thoughtsTokenCount,
		OutputTokenCount:   outputTokenCount,
		TotalTokenCount:    totalTokenCount,
		IsTest:             false,
	}, nil

}

func (GoogleLLMNoteProcessor) TextParse(text string, modelName LLMModelName) (*LLMResponse, error) {
	ctx := context.Background()
	// fileName
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		//GOOGLE_API_KEY env
		APIKey:  os.Getenv("GOOGLE_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	// Upload each file and get the URI

	// donot use html elements such as sub sup
	// Analyze the image text and pay special attention to precision. We need to return:
	// 1. Proper complete image text with original transcript language (with accompanying illustration explanation in braces if exists). Ensure transcript precision and technical context is consistent and correct and also format it to have correct commas, quotes, parentheses and generally proper readable text, return only transcription, no timestamps. Additionally, For each highly unclear word provide placeholder of possible options and their possibility threshold in form of this example: "  [gone:0.7][done:0.5] " meaning that we are not sure about this word/phrase.
	// 2. Concise subject name on original text content language only.
	// 3. Markdown Format Summary in original content language: Start with brief introduction then specify key points, events, facts. You can generate a table for similar key facts.
	// If image is empty or not understandable, please return only "Unknown note" for the name and keep other fields empty.
	// 4. Quiz Object: Generate quiz with 10 questions: - "question": a string with the question text - "options": an array of 4 distinct string options - "answer": the correct string from the options array
	// Ensure questions accurately reflect the image content.```
	// Return the result in JSON format.
	var parts []*genai.Part

	parts = append(parts, &genai.Part{
		Text: text,
	})
	if err != nil {
		log.Fatal(err)
	}
	//Please use ONLY the following JSON schema for the response:
	//Note = {'name': string, 'md_summary': string, 'transcription': string, 'quiz_json': Array<{'question': string, 'answer': string, 'options': Array<string>}>}
	//Return: Note},
	// result, err := client.Models.GenerateContent(ctx, "gemini-2.5-pro-preview-03-25", []*genai.Content{{Parts: parts}}, nil)
	result, err := client.Models.GenerateContent(ctx, modelName.String(), []*genai.Content{{Parts: parts}}, &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: `Check the given text for small grammatical and context errors.
We need to return JSON data with the following fields and rules:
1. Concise subject name only on original text language.
2. Return original text using Markdown but with correct grammar, punctuation, and formatting in the original language, adding commas and quotes where needed for clarity. Ensure complete sentences and a readable layout. For words or phrases clearly out of context, replace with a placeholder and provide three options: the original word/phrase (first), and two alternatives, with possibility thresholds in the format original:probability][alternative1:probability][alternative2:probability], e.g., [original:0.4][gone:0.7][done:0.5]. Apply placeholders only for definitively out-of-context words/phrases.
3. Summarize in the original texts language using Markdown. Begin with a brief introduction, followed by key points, events, and facts. Use a table for related key facts. Avoid nested lists. Only these markdown directives allowed if needed: bold, table elements, heading1, heading2, heading3, line breaks, new lines, emphasis, italic, blockquotes, any lists (not nested), code blocks, inline code
4. Text language in ISO 639 format.`},
			},
		},
		ResponseSchema: &genai.Schema{
			Type: "object",
			// The schema for the AI response.
			Properties: map[string]*genai.Schema{
				"name": {
					Type: "string",
				},
				"md_summary": {
					Type: "string",
				},
				"transcription": {
					Type: "string",
				},
				"language": {
					Type: "string",
				},
			},
			Required: []string{"name", "md_summary", "transcription", "language"},
		},
	})

	// client.Models.Cou

	if err != nil {
		fmt.Println("Error in GenerateContent:", err)
		return nil, fmt.Errorf("%v", err)
	}
	var inputTokenCount int32
	var thoughtsTokenCount int32
	var outputTokenCount int32
	var totalTokenCount int32
	if result.UsageMetadata != nil {
		inputTokenCount = result.UsageMetadata.PromptTokenCount
		thoughtsTokenCount = result.UsageMetadata.ThoughtsTokenCount
		outputTokenCount = result.UsageMetadata.CandidatesTokenCount
		totalTokenCount = result.UsageMetadata.TotalTokenCount
	} else {
		fmt.Println("UsageMetadata is nil!")
	}
	llmResponseText, err := GetFirstCandidateTextWithThoughts(result)
	if err != nil {
		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	}
	return &LLMResponse{
		Response:           llmResponseText.Text,
		Thoughts:           llmResponseText.Thoughts,
		InputTokenCount:    inputTokenCount,
		ThoughtsTokenCount: thoughtsTokenCount,
		OutputTokenCount:   outputTokenCount,
		TotalTokenCount:    totalTokenCount,
		IsTest:             false,
	}, nil

}

func (GoogleLLMNoteProcessor) ExamParse(text *string, filePaths []string, modelName LLMModelName) (*LLMResponse, error) {
	ctx := context.Background()
	// fileName
	fmt.Println("File path for image parse:", filePaths)
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		//GOOGLE_API_KEY env
		APIKey:  os.Getenv("GOOGLE_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	var genFiles []*genai.File
	// Upload each file and get the URI
	for _, filePath := range filePaths {
		// fmt.Println("File path for image parse:", filePath)
		genFile, err := client.Files.UploadFromPath(ctx, filePath, &genai.UploadFileConfig{})
		if err != nil {
			return nil, fmt.Errorf("error uploading GEN AI image file: %s %v", filePath, err)
		}
		genFiles = append(genFiles, genFile)
	}

	var parts []*genai.Part
	// generate pars from for each file then merge it with text
	for i, genFile := range genFiles {
		fmt.Println("File path for image parse:", i, " ", genFile.URI, genFile.MIMEType)
		parts = append(parts, &genai.Part{
			FileData: &genai.FileData{
				FileURI:  genFile.URI,
				MIMEType: genFile.MIMEType,
			},
		})
	}
	if text != nil {
		parts = append(parts, &genai.Part{
			Text: *text,
		})
	}
	if err != nil {
		log.Fatal(err)
	}
	//Please use ONLY the following JSON schema for the response:
	//Note = {'name': string, 'md_summary': string, 'transcription': string, 'quiz_json': Array<{'question': string, 'answer': string, 'options': Array<string>}>}
	//Return: Note},
	// result, err := client.Models.GenerateContent(ctx, "gemini-2.5-pro-preview-03-25", []*genai.Content{{Parts: parts}}, nil)
	result, err := client.Models.GenerateContent(ctx, modelName.String(), []*genai.Content{{Parts: parts}}, &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: `You are tasked with analyzing provided study material (text and images, if any) to create a structured test transcript and a summary based on the content. The output must strictly adhere to the requirements below. Do not solve the tests; only transcribe and summarize them.

### Requirements

1. **Structured Test Transcript**
   - Transcribe the test or exam questions exactly as provided in the study material, using Markdown with correct grammar, punctuation, and formatting in the original language.
   - Add commas, quotes, and other punctuation where needed for clarity.
   - For graphs and non-table illustrations use ASCII art or simple text-based representations. Triple check the accuracy of the transcription.
   - Use complete sentences and ensure a readable layout optimized for mobile devices.
   - Include all questions, even if open-ended or with variants (e.g., multiple-choice, fill-in-the-blank, or essay questions).
   - For illustrations, provide a brief description in the original language (e.g., "Diagram of a triangle with labeled angles").
   - Use only the following Markdown directives: bold ('**'), table elements ('|', '-', '|'), headings ('#', '##', '###'), line breaks, new lines, emphasis/italic ('*'), blockquotes ('>'), unordered lists ('-'), ordered lists ('1.'), code blocks , inline code.
   - Do not use LaTeX, HTML elements, or nested lists.
   - Example layouts for test questions:
     ` + "```markdown" + `
     # Mathematics Test

     ## Question 1
     Solve the equation: 2x + 3 = 7.

     ## Question 2
     What is the area of a circle with radius 5 cm? (Use π = 3.14)

     ## Question 3 (Multiple Choice)
     Which of the following is a prime number?
     - A) 4
     - B) 7
     - C) 9
     - D) 15

     ## Question 4 (Open-Ended)
     Explain the Pythagorean theorem in your own words.

     ## Question 5
     | Angle | Measure |
     |-------|---------|
     | A     | 30°     |
     | B     | 60°     |
     | C     | ?       |
     Given the triangle above, calculate the measure of angle C. (Illustration: Triangle with angles A, B, and C labeled)

     ## Question 6 (ASCII Illustration)
     Consider the following circuit:
     ` + "```" + `
     [Battery] ---- [Resistor] ---- [Bulb]
     ` + "```" + `
     Describe the flow of current in this circuit.
     ` + "```" + `
	
	 ## Question 7 (Open - Multiple linking options and answers)
	 Define relationship between the following terms:
	 1. Photosynthesis
	 2. Respiration
	 3. Energy conversion
	 a. Photosynthesis is the process by which green plants and some other organisms use sunlight to synthesize foods with the help of chlorophyll pigments.
	 b. Respiration is the process of breaking down glucose to release energy.
	 c. Energy conversion is the process of changing energy from one form to another.

2. **Test or Exam Subject Name**
   - Provide a concise subject name in the original language based on the content (e.g., "Álgebra Básica" for a Spanish algebra test).
   - If the content is empty or not understandable, return only "Unknown note".

3. **Summary**
   - Write a summary in the original language using Markdown, without LaTeX.
   - Structure the summary as follows:
     - **Subject word in original transacript translation**: A short, generic explanation of the topic (1-2 sentences).
     - **'Topics Covered' phrase in original transacript translation**: List the specific subjects or skills tested (e.g., linear equations, geometry).
     - **'Conclusion' phrase in original transacript translation**: Mention the complexity (e.g., beginner, intermediate, advanced) and recommend 1-2 study books or resources commonly used for the topics.
   - Use only the allowed Markdown directives (same as for the transcript).
   - Example:
 ` + "```markdown" + `
     # Summary

     **Subject**: Basic algebra focuses on solving equations and understanding variables.

     **Topics Covered**:
     - Solving linear equations
     - Properties of exponents
     - Quadratic equations

     **Conclusion**: This test is at an intermediate level. Recommended study books include "Algebra I For Dummies" by Mary Jane Sterling and "Intermediate Algebra" by Charles P. McKeague.
     ` + "```" + `

4. **Content Language**
   - Specify the language of the content in ISO 639-1 format (2 symbols, e.g., 'en' for English, 'es' for Spanish).
   - If the language is unclear, use 'und' (undetermined).

### Additional Instructions
- If the study material is empty or incomprehensible, return only the subject name as "Unknown note" and leave other fields empty.
- Ensure all questions and content are transcribed verbatim, preserving the original intent and wording.
- For tables, ensure proper alignment and clear headers.
- For open-ended questions or those with variants, include all options or instructions as presented.
- Double-check that no LaTeX or HTML is used, and avoid nested lists.
- The output must be complete, with no truncated content or missing sections.

`}}},
		ResponseSchema: &genai.Schema{
			Type: "object",
			// The schema for the AI response.
			Properties: map[string]*genai.Schema{
				"name": {
					Type: "string",
				},
				"md_summary": {
					Type: "string",
				},
				"transcription": {
					Type: "string",
				},
				"language": {
					Type: "string",
				},
			},
			Required: []string{"name", "md_summary", "transcription", "language"},
		},
		ThinkingConfig: &genai.ThinkingConfig{
			IncludeThoughts: true,
			// ThinkingBudget:  Int32Pointer()
		},
	})

	// client.Models.Cou

	if err != nil {
		fmt.Println("Error in GenerateContent:", err)
		return nil, fmt.Errorf("%v", err)
	}
	var inputTokenCount int32
	var thoughtsTokenCount int32
	var outputTokenCount int32
	var totalTokenCount int32
	if result.UsageMetadata != nil {
		inputTokenCount = result.UsageMetadata.PromptTokenCount
		thoughtsTokenCount = result.UsageMetadata.ThoughtsTokenCount
		outputTokenCount = result.UsageMetadata.CandidatesTokenCount
		totalTokenCount = result.UsageMetadata.TotalTokenCount
	} else {
		fmt.Println("UsageMetadata is nil!")
	}
	llmResponseText, err := GetFirstCandidateTextWithThoughts(result)
	if err != nil {
		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	}
	return &LLMResponse{
		Response:           llmResponseText.Text,
		Thoughts:           llmResponseText.Thoughts,
		InputTokenCount:    inputTokenCount,
		ThoughtsTokenCount: thoughtsTokenCount,
		OutputTokenCount:   outputTokenCount,
		TotalTokenCount:    totalTokenCount,
		IsTest:             false,
	}, nil

}

func (GoogleLLMNoteProcessor) GenerateQuizAndFlashCards(content string, isSourceTest bool, modelName LLMModelName, language string) (*LLMResponse, error) {
	ctx := context.Background()
	// fileName
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		//GOOGLE_API_KEY env
		APIKey:  os.Getenv("GOOGLE_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	// Upload each file and get the URI

	// donot use html elements such as sub sup
	// Analyze the image text and pay special attention to precision. We need to return:
	// 1. Proper complete image text with original transcript language (with accompanying illustration explanation in braces if exists). Ensure transcript precision and technical context is consistent and correct and also format it to have correct commas, quotes, parentheses and generally proper readable text, return only transcription, no timestamps. Additionally, For each highly unclear word provide placeholder of possible options and their possibility threshold in form of this example: "  [gone:0.7][done:0.5] " meaning that we are not sure about this word/phrase.
	// 2. Concise subject name on original text content language only.
	// 3. Markdown Format Summary in original content language: Start with brief introduction then specify key points, events, facts. You can generate a table for similar key facts.
	// If image is empty or not understandable, please return only "Unknown note" for the name and keep other fields empty.
	// 4. Quiz Object: Generate quiz with 10 questions: - "question": a string with the question text - "options": an array of 4 distinct string options - "answer": the correct string from the options array
	// Ensure questions accurately reflect the image content.```
	// Return the result in JSON format.
	var parts []*genai.Part

	parts = append(parts, &genai.Part{
		Text: content,
	})
	if err != nil {
		log.Fatal(err)
	}

	systemPrompt := `You are tasked with generating a JSON object based on the provided study material. The JSON must strictly adhere to the following specific requirements, and you must double-check the correctness of all questions, answers, and explanations to ensure accuracy. Do not deviate from these instructions, as they are deliberately specific. Assume the material contains technical content, including potential graphs, illustrations, or tests, and follow these instructions precisely.
### Requirements for the JSON Output:
1. **Questions Section**:
   - Generate **10 easy questions** and **10 hard questions** and **10 bonus advanced questions** based on the provided study material in "` + language + `" language (ISO 639-1 code).
   - Each question must:
     - Be written in markdown format if formatting (e.g., bold, italics, new lines or code) is needed for clarity.
     - Include an array of **4 non-markdown string options** (plain text, no markdown formatting in the options themselves). Only 1 option should have correct answer all other (3) options should be wrong.
     - Have the **correct answer index** ('0', '1', '2', or '3') corresponding to one of the options. Not the answer string.
     - Provide a **concise explanation** (1-2 sentences) explaining why the correct answer is correct, written in plain text (no markdown).
   - 2 of the questions should be slightly out of material context but still related to the material.
   - For critical technical questions involving graphs or illustrations, represent them using ASCII symbols or text-based descriptions (e.g., a table or diagram made with  - ,  | ,  +).
   - For questions derived from specific parts of the material (e.g., tests or examples), explicitly reference the relevant section or question (e.g., "Based on Test 1, Question 3").
   - Do **not** solve input tests if included in the material; use them only as a reference for question creation.
   - Ensure all questions are clear, unambiguous, and directly related to the material.

2. **Flashcards Section**:
   - Generate **10 flashcards** based on the study material in "` + language + `" language (ISO 639-1 code).
   - Each flashcard must have:
     - A **question side** (non-markdown, plain text).
     - An **answer side** (non-markdown, plain text).
   - The content must be in the **original language of the material**.
   - Ensure flashcards cover key concepts, definitions, or facts from the material.

3. **General Instructions**:
   - **Do not use LaTeX** for any mathematical expressions. Use plain text or simple symbols (e.g., x^2 for x squared, sqrt(x) for square root).
   - **Double-check** the correctness of all questions, options, answers, and explanations to ensure they are accurate and consistent with the material.
   - Ensure the JSON contains exactly **10 easy questions**, **10 hard questions**, and **10 flashcards**.
   - If the material is insufficient to generate all required questions or flashcards, prioritize quality over quantity but aim to meet the exact counts.
   - If graphs or illustrations are needed but cannot be represented textually, rephrase the question to avoid requiring a visual while still testing the same concept.

### Notes for Execution:
- Ensure all content is derived from the provided study material, maintaining fidelity to its concepts, terminology, and language.
- Avoid generating questions or flashcards that are overly vague, repetitive, or unrelated to the material.
- If the material includes multiple topics, distribute questions and flashcards evenly across them to ensure comprehensive coverage.
`

	if isSourceTest {
		systemPrompt = `You are tasked with generating a JSON object based on the provided test or exam material. The JSON must strictly adhere to the following specific requirements, and you must double-check the correctness of all questions, answers, and explanations to ensure accuracy. Do not deviate from these instructions, as they are deliberately specific. Assume the material contains technical content, including potential graphs, illustrations, or tests, and follow these instructions precisely.
### Requirements for the JSON Output:
1. **Questions Section**:
   - Generate **10 easy questions** and **10 hard questions** based on the provided study material.
   - Each question must:
	 - Should be similar to the original questions but not identical. It can be a variation or a different derived question based on each question.
     - Be written in markdown format if formatting (e.g., bold, italics, new lines or code) is needed for clarity.
     - Include an array of **4 non-markdown string options** (plain text, no markdown formatting in the options themselves). Only 1 option should have correct answer all other (3) options should be wrong.
     - Have the **correct answer index** ('0', '1', '2', or '3') corresponding to one of the options. Not the answer string.
     - Provide a **concise explanation** (1-2 sentences) explaining why the correct answer is correct, written in plain text (no markdown).
   - 2 of the questions should be slightly out of material context but still related to the material.
   - For critical technical questions involving graphs or illustrations, represent them using ASCII symbols or text-based descriptions (e.g., a table or diagram made with  - ,  | ,  +).
   - For questions **ONLY** that needs a context because it is derived from specific parts of the material (e.g., tests or examples), explicitly reference the relevant section or question (e.g., "Based on Test 1, Question 3") .
   - Do **not** solve input tests if included in the material; use them only as a reference for question creation.
   - Ensure all questions are clear, unambiguous, and directly related to the material.

2. **Flashcards Section**:
   - Generate **10 flashcards** based on the study material.
   - Each flashcard must have:
     - A **question side** (non-markdown, plain text).
     - An **answer side** (non-markdown, plain text).
   - The content must be in the **original language of the material**.
   - Ensure flashcards cover key concepts, definitions, or facts from the material.

3. **General Instructions**:
   - **Do not use LaTeX** for any mathematical expressions. Use plain text or simple symbols (e.g., x^2 for x squared, sqrt(x) for square root).
   - **Double-check** the correctness of all questions, options, answers, and explanations to ensure they are accurate and consistent with the material.
   - Ensure the JSON contains exactly **10 easy questions**, **10 hard questions**, and **10 flashcards**.
   - If the material is insufficient to generate all required questions or flashcards, prioritize quality over quantity but aim to meet the exact counts.
   - If graphs or illustrations are needed but cannot be represented textually, rephrase the question to avoid requiring a visual while still testing the same concept.

### Notes for Execution:
- Ensure all content is derived from the provided study material, maintaining fidelity to its concepts, terminology, and language.
- Avoid generating questions or flashcards that are overly vague, repetitive, or unrelated to the material.
- If the material includes multiple topics, distribute questions and flashcards evenly across them to ensure comprehensive coverage.
`
	}
	//Please use ONLY the following JSON schema for the response:
	//Note = {'name': string, 'md_summary': string, 'transcription': string, 'quiz_json': Array<{'question': string, 'answer': string, 'options': Array<string>}>}
	//Return: Note},
	// result, err := client.Models.GenerateContent(ctx, "gemini-2.5-pro-preview-03-25", []*genai.Content{{Parts: parts}}, nil)
	result, err := client.Models.GenerateContent(ctx, modelName.String(), []*genai.Content{{Parts: parts}}, &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: systemPrompt},
			},
		},
		ResponseSchema: &genai.Schema{
			Type: "object",
			// The schema for the AI response.
			Properties: map[string]*genai.Schema{

				"easy_questions": {
					Type:      "array",
					MaxLength: Int64Pointer(10),
					MinLength: Int64Pointer(10),
					Items: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"question": {
								Type: "string",
							},
							"answer": {
								Type: "integer",
							},
							"explanation": {
								Type: "string",
							},
							"options": {
								Type: "array",
								Items: &genai.Schema{
									Type:      "string",
									MaxLength: Int64Pointer(4),
									MinLength: Int64Pointer(4),
								},
							},
						},
						Required: []string{"question", "answer", "explanation", "options"},
					},
				},
				"hard_questions": {
					Type:      "array",
					MaxLength: Int64Pointer(10),
					MinLength: Int64Pointer(10),
					Items: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"question": {
								Type: "string",
							},
							"answer": {
								Type: "integer",
							},
							"explanation": {
								Type: "string",
							},
							"options": {
								Type: "array",
								Items: &genai.Schema{
									Type:      "string",
									MaxLength: Int64Pointer(4),
									MinLength: Int64Pointer(4),
								},
							},
						},
						Required: []string{"question", "answer", "explanation", "options"},
					},
				},
				"bonus_questions": {
					Type:      "array",
					MaxLength: Int64Pointer(10),
					MinLength: Int64Pointer(10),
					Items: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"question": {
								Type: "string",
							},
							"answer": {
								Type: "integer",
							},
							"explanation": {
								Type: "string",
							},
							"options": {
								Type: "array",
								Items: &genai.Schema{
									Type:      "string",
									MaxLength: Int64Pointer(4),
									MinLength: Int64Pointer(4),
								},
							},
						},
						Required: []string{"question", "answer", "explanation", "options"},
					},
				},
				"flashcards": {
					Type:      "array",
					MaxLength: Int64Pointer(10),
					MinLength: Int64Pointer(10),
					Items: &genai.Schema{
						Type: "object",
						Properties: map[string]*genai.Schema{
							"question": {
								Type: "string",
							},
							"answer": {
								Type: "string",
							},
						},
						Required: []string{"question", "answer"},
					},
				},
			},
			Required: []string{"easy_questions", "hard_questions", "flashcards"},
		},
	})

	// client.Models.Cou

	if err != nil {
		fmt.Println("Error in GenerateContent:", err)
		return nil, fmt.Errorf("%v", err)
	}
	var inputTokenCount int32
	var thoughtsTokenCount int32
	var outputTokenCount int32
	var totalTokenCount int32
	if result.UsageMetadata != nil {
		inputTokenCount = result.UsageMetadata.PromptTokenCount
		thoughtsTokenCount = result.UsageMetadata.ThoughtsTokenCount
		outputTokenCount = result.UsageMetadata.CandidatesTokenCount
		totalTokenCount = result.UsageMetadata.TotalTokenCount
	} else {
		fmt.Println("UsageMetadata is nil!")
	}
	llmResponseText, err := GetFirstCandidateTextWithThoughts(result)
	if err != nil {
		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	}
	return &LLMResponse{
		Response:           llmResponseText.Text,
		InputTokenCount:    inputTokenCount,
		Thoughts:           llmResponseText.Thoughts,
		ThoughtsTokenCount: thoughtsTokenCount,
		OutputTokenCount:   outputTokenCount,
		TotalTokenCount:    totalTokenCount,
		IsTest:             false,
	}, nil

}
