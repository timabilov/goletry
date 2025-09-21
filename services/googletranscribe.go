package services

import (
	"context"
	"fmt"
	"log"
	"os"
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
	Flash25Image
	Seedream40
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
	case Flash25Image:
		return "gemini-2.5-flash-image-preview"
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
	Response           string   `json:"response"`
	Images             [][]byte `json:"images,omitempty"`
	InputTokenCount    int32    `json:"input_token_count"`
	Thoughts           string   `json:"thoughts"`
	ThoughtsTokenCount int32    `json:"thoughts_token_count"`
	OutputTokenCount   int32    `json:"output_token_count"`
	TotalTokenCount    int32    `json:"total_token_count"`
	IsTest             bool     `json:"is_test"`
	// to add
}

type LLMProcessor interface {
	ProcessClothing(filePath string, modelName LLMModelName) (*LLMResponse, error)
	ProcessAvatarTask(personAvatarPath string, modelName LLMModelName) (*LLMResponse, error)
	GenerateTryOn(personAvatarPath string, filePaths []string, modelName LLMModelName) (*LLMResponse, error)
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

func GetAllInlineImages(result *genai.GenerateContentResponse) ([][]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("cannot  response")
	}

	var allImageData [][]byte

	for _, cand := range result.Candidates {
		// It's good practice to check safety ratings first.
		for _, rating := range cand.SafetyRatings {
			if rating.Blocked {
				return nil, fmt.Errorf("content blocked by safety setting: %s", rating.Category)
			}
		}
		if cand.Content == nil || len(cand.Content.Parts) == 0 {
			continue // This candidate has no content, skip to the next.
		}

		for _, part := range cand.Content.Parts {
			// Type-assert the part to see if it's InlineData.
			inlineData := part.InlineData
			if inlineData != nil {
				// Check if the MIME type indicates it's an image.
				if strings.HasPrefix(inlineData.MIMEType, "image/") {
					// Ensure there's actually data before appending.
					if len(inlineData.Data) > 0 {
						allImageData = append(allImageData, inlineData.Data)
					}
				}
			}
		}
	}

	// If the loop completes and we found no images, return nil as requested.
	if len(allImageData) == 0 {
		return nil, nil
	}

	return allImageData, nil
}
func GetFirstCandidateTextWithThoughts(result *genai.GenerateContentResponse) (*ResponseWithThoughts, error) {
	var thinkingContent string
	for _, c := range result.Candidates {
		// fmt.Println("Candidate:", i, c.Content.Parts[0].Text, c.Content.Parts[0].Thought)
		fmt.Println("Finish reason: ", c.FinishReason, " Finish message: ", c.FinishMessage)

		if len(c.SafetyRatings) > 0 {
			fmt.Println("[Safety] Safety ratings present:", len(c.SafetyRatings))
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
	return &ResponseWithThoughts{
		Thoughts: thinkingContent,
		Text:     result.Text(),
	}, nil
}

func (GoogleLLMNoteProcessor) ProcessAvatarTask(personAvatarPath string, modelName LLMModelName) (*LLMResponse, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GOOGLE_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	// This file must exist in the same folder as your executable.
	const whiteCanvasPath = "./white_540x960.png"
	_, err = os.Open(whiteCanvasPath)
	if err != nil {
		return nil, err
	}
	// osf.Re
	var genFiles []*genai.File

	// 1. Upload the user's avatar
	personAvatarFile, err := tryUploadGoogleStorage(ctx, client, personAvatarPath, nil)
	if err != nil {
		fmt.Println("Error uploading person avatar file:", personAvatarPath, err)
		return nil, fmt.Errorf("error uploading person avatar file %s: %v", personAvatarPath, err)
	}
	genFiles = append(genFiles, personAvatarFile)
	fmt.Println("Successfully uploaded person avatar:", personAvatarPath)

	whiteCanvasFile, err := tryUploadGoogleStorage(ctx, client, whiteCanvasPath, nil)
	if err != nil {
		fmt.Println("Error uploading white canvas file:", whiteCanvasPath, err)
		return nil, fmt.Errorf("error uploading white canvas file %s: %v", whiteCanvasPath, err)
	}
	genFiles = append(genFiles, whiteCanvasFile)
	fmt.Println("Successfully uploaded white canvas:", whiteCanvasPath)

	// [Image1, Image2, Text]
	var parts []*genai.Part

	// First, add all the image file parts.
	for _, genFile := range genFiles {
		fmt.Println("Adding image part for:", genFile.URI)
		parts = append(parts, &genai.Part{
			FileData: &genai.FileData{
				FileURI:  genFile.URI,
				MIMEType: genFile.MIMEType,
			},
		})
	}

	// Second, add the text prompt part at the end.
	// The prompt correctly refers to the "second image" as the background.
	parts = append(parts, &genai.Part{
		//Generate a hyper-realistic fashion-style full-body commercial head to toe portrait of the  person from first image by keeping his identity, personality, facial identity(100% same) and use solid, flat, unlit, white second image as a new background for person image which will be chromakey. keep user facial identity exactly same, unchanged. User should be standing as in fashion e-commerce photo-shoot  pose with neutral white shirt, white trousers and white neutral shoes.. The lighting on user should be natural and professional, high-resolution. Remove items from hands, position neutrally with slight smile. Clean all background elements, watermarks, other people/objects. If no person detected: return "NO_PERSON", otherwise output only full-body person, with on flat, consistent, all white second image background. Do not apply slight grayish gradients, keep all edges white. Aspect ratio 9:16 portrait size
		//Generate a hyper-realistic fashion-style full-body commercial head to toe photographer edited portrait of the  person from first image by keeping his identity, personality, facial identity(100% same) and use solid, flat, unlit, white second image as a new background for person image which will be chromakey. keep user facial identity exactly same, unchanged. By keeping same personality, identity and exact same body sizes - person should be standing straight facing the camera in RELAXED cool pose with neutral white shirt, white trousers and white neutral shoes. The lighting on user should be natural, soft and professional, high-resolution and opening the color of person. Remove items from hands, position neutrally with slight smile. Clean all background elements, watermarks, other people/objects. If no person detected: return "NO_PERSON", otherwise output only full-body person, with on flat, consistent, all white second image background. Do not apply slight grayish gradients, keep all edges white. Aspect ratio 9:16 portrait size
		//Generate a fashion-style full-body commercial head to toe photographer edited portrait of the  person from first image by keeping his identity, personality, facial identity(100% same) and use solid, flat, unlit, white second image as a new background for person image which will be chromakey. keep user facial identity exactly same, unchanged. By keeping same personality, identity and exact same body/hand/head/leg sizes - person should be standing straight facing the camera in RELAXED cool pose with neutral white shirt, white trousers and white neutral shoes. The lighting on user should be natural, soft and professional, high-resolution and opening the color of person. Remove items from hands, position neutrally with slight smile. Clean all background elements, watermarks, other people/objects. If no person detected: return "NO_PERSON", otherwise output only full-body person, with on flat, consistent, all white second image background. Do not apply slight grayish gradients, keep all edges white. Aspect ratio 9:16 portrait size
		//Generate a fashion-style full-body commercial head to toe photographer edited portrait of the  person from first image by keeping his identity, personality, facial identity(100% same) and use solid, flat, unlit, white second image as a new background for person image which will be chromakey. keep user facial identity exactly same, unchanged. By keeping same personality, identity and exact same body/hand/head/leg sizes - person should be standing straight facing the camera in relaxed, coolest, confident pose with neutral white shirt, white trousers and white neutral shoes. The lighting on user should be natural, soft and professional, high-resolution and opening the color of person. Remove items from hands, position neutrally with slight smile. Clean all background elements, watermarks, other people/objects. If no person detected: return "NO_PERSON", otherwise output only full-body person, with on flat, consistent, all white second image background. Do not apply slight grayish gradients, keep all edges white. Aspect ratio 9:16 portrait size
		// Generate a fashion-style full-body commercial head to toe photographer edited portrait of the  person from first image by keeping his identity, personality, facial identity(100% same) and use solid, flat, unlit, white second image as a new background for person image which will be chromakey. keep user facial identity exactly same, unchanged. By keeping same personality, identity and exact same body/hand/head/leg/hair sizes - generate the straight facing the camera and relaxed, coolest, confident pose with neutral white shirt, white trousers and white neutral shoes. The lighting on user should be natural, soft and professional, high-resolution and opening the color of person. Remove items from hands, position neutrally with slight smile. Clean all background elements, watermarks, other people/objects. If no person detected: return "NO_PERSON", otherwise output only full-body person, with on flat, consistent, all white second image background. Do not apply slight grayish gradients, keep all edges white. Aspect ratio 9:16 portrait size
		Text: "Generate a fashion-style full-body commercial head to toe photographer edited portrait of the person from first image by keeping his identity, personality, facial identity(100% same) and use solid, flat, unlit, white second image as a new background for person image which will be chromakey. keep user facial identity exactly same, unchanged. Person should be in center and should take 70% of the image area. By keeping same personality, identity and exact same body/hand/head/leg sizes - generate the straight facing the camera and relaxed, coolest, confident pose with neutral white shirt, white trousers and white neutral shoes. The lighting on user should be natural, soft and professional, high-resolution and opening the color of person. Remove items from hands, position neutrally with slight smile. Clean all background elements, watermarks, other people/objects. If no person detected: return \"NO_PERSON\", otherwise output only full-body person, with on flat, consistent, all white second image background. Do not apply slight grayish gradients, keep all edges white. Aspect ratio 9:16 portrait size",
	})

	// The rest of your function remains the same...
	result, err := client.Models.GenerateContent(ctx, modelName.String(), []*genai.Content{{Parts: parts}}, &genai.GenerateContentConfig{
		MaxOutputTokens: 50000,
		Temperature:     floatPointer(1),
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: `If no person detected in the image return NO_PERSON as response. Analyze the image, and provide only an full body avatar.`},
			},
		},
	})

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

	if result.PromptFeedback != nil {
		fmt.Println(result.PromptFeedback.BlockReason)
		fmt.Println(result.PromptFeedback.BlockReasonMessage)
		fmt.Println(result.PromptFeedback.SafetyRatings)
		return nil, fmt.Errorf("content violation: %s %s ", personAvatarPath, result.PromptFeedback.BlockReasonMessage)
	}

	fmt.Println("Number of candidates received:", len(result.Candidates))
	llmResponseImagesBytes, err := GetAllInlineImages(result)
	if err != nil {
		fmt.Println("Error getting first candidate image: ", err)
		fmt.Println(result)
		return nil, fmt.Errorf("error getting first candidate image: %v", err)
	}

	fmt.Println("Number of images extracted:", len(llmResponseImagesBytes))
	llmResponseText, err := GetFirstCandidateTextWithThoughts(result)
	if err != nil {
		fmt.Println("Error getting first candidate text: ", err)
		fmt.Println(result.Candidates)
		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	}

	return &LLMResponse{
		Response:           llmResponseText.Text,
		Images:             llmResponseImagesBytes,
		Thoughts:           llmResponseText.Thoughts,
		InputTokenCount:    inputTokenCount,
		ThoughtsTokenCount: thoughtsTokenCount,
		OutputTokenCount:   outpuTokenCount,
		TotalTokenCount:    totalTokenCount,
		IsTest:             false,
	}, nil
}

func (GoogleLLMNoteProcessor) GenerateTryOn(personAvatarPath string, filePaths []string, modelName LLMModelName) (*LLMResponse, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GOOGLE_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	// filter null and keep only existing images in filePaths,rewrite

	var genFiles []*genai.File

	genFile, err := tryUploadGoogleStorage(ctx, client, personAvatarPath, nil)
	if err != nil {
		fmt.Println("Error uploading person avatar file:", personAvatarPath, err)
		return nil, fmt.Errorf("error uploading file %s: %v", personAvatarPath, err)
	}
	genFiles = append(genFiles, genFile)
	// Upload each file and get the URI
	for i, filePath := range filePaths {
		if filePath == "" {
			fmt.Println("File path empty in index:", i)
			continue
		}
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

	result, err := client.Models.GenerateContent(ctx, modelName.String(), []*genai.Content{{Parts: parts}}, &genai.GenerateContentConfig{
		// ResponseMIMEType: "application/json",
		CandidateCount: 1,
		// ThinkingConfig: &genai.ThinkingConfig{
		// 	IncludeThoughts: true,
		// 	ThinkingBudget:  Int32Pointer(3000),
		// },
		// because its youtube it can have more..
		MaxOutputTokens: 50000,
		Temperature:     floatPointer(1),
		// TopK:            floatPointer(0.5),
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: `Edit first person image into a fashion-style full-body commercial head to toe photographer edited by keeping his identity, personality, placement in image in center, facial identity(100% same) and use the same solid, flat, unlit, white first image background including ratio. Take the all images after first one and let the same exact person from the first image wear it. For missing clothing items, keep original ones that user wears. keep user facial identity exactly same, unchanged. By keeping same personality, identity and exact same body/hand/head/leg sizes - generate the straight facing the camera and relaxed, coolest, confident pose with neutral white shirt, white trousers and white neutral shoes. The lighting on user should be natural, soft and professional, high-resolution and opening the color of person. Remove items from hands, position neutrally with slight smile. Clean all background elements, watermarks, other people/objects. If no person detected: return "NO_PERSON", otherwise output only full-body person, with on flat, consistent, all white second image background. Do not apply slight grayish gradients, keep all edges white. Aspect ratio 9:16 portrait size`},
			},
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
	if result.PromptFeedback != nil {

		fmt.Println(result.PromptFeedback.BlockReason)
		fmt.Println(result.PromptFeedback.BlockReasonMessage)
		fmt.Println(result.PromptFeedback.SafetyRatings)
		return nil, fmt.Errorf("content violation: %s %s ", filePaths[0], result.PromptFeedback.BlockReasonMessage)
	}
	fmt.Println("Number of candidates received:", len(result.Candidates))
	llmResponseImagesBytes, err := GetAllInlineImages(result)
	if err != nil {
		fmt.Println("Error getting first candidate image: ", err)

		fmt.Println(result)

		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	}
	fmt.Println("Number of images extracted:", len(llmResponseImagesBytes))
	llmResponseText, err := GetFirstCandidateTextWithThoughts(result)
	if err != nil {
		fmt.Println("Error getting first candidate text: ", err)

		fmt.Println(result.Candidates)
		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	}
	// fmt.Pri
	return &LLMResponse{
		Response:           llmResponseText.Text,
		Images:             llmResponseImagesBytes,
		Thoughts:           llmResponseText.Thoughts,
		InputTokenCount:    inputTokenCount,
		ThoughtsTokenCount: thoughtsTokenCount,
		OutputTokenCount:   outpuTokenCount,
		TotalTokenCount:    totalTokenCount,
		IsTest:             false,
	}, nil

}

var dashAlphaRule = regexp.MustCompile(`[^a-zA-Z0-9-]`)

func (GoogleLLMNoteProcessor) ProcessClothing(filePath string, modelName LLMModelName) (*LLMResponse, error) {
	return nil, nil
	// 	ctx := context.Background()
	// 	// fileName
	// 	fmt.Println("File path for image parse:", filePaths)
	// 	client, err := genai.NewClient(ctx, &genai.ClientConfig{
	// 		//GOOGLE_API_KEY env
	// 		APIKey:  os.Getenv("GOOGLE_API_KEY"),
	// 		Backend: genai.BackendGeminiAPI,
	// 	})
	// 	var genFiles []*genai.File
	// 	// Upload each file and get the URI
	// 	for _, filePath := range filePaths {
	// 		// fmt.Println("File path for image parse:", filePath)
	// 		fileName := filepath.Base(filePath)
	// 		sanitizedFileName := dashAlphaRule.ReplaceAllString(strings.ReplaceAll(fileName, ".", "-"), "")
	// 		genFile, err := tryUploadGoogleStorage(ctx, client, filePath, &sanitizedFileName)
	// 		if err != nil {
	// 			fmt.Println("Error uploading file:", filePath, err)
	// 			return nil, fmt.Errorf("error uploading file to google storage %s: %v", filePath, err)
	// 		}
	// 		genFiles = append(genFiles, genFile)
	// 	}

	// 	var parts []*genai.Part
	// 	// generate pars from for each file then merge it with text
	// 	for i, genFile := range genFiles {
	// 		fmt.Println("File path for image parse:", i, " ", genFile.URI, genFile.MIMEType)
	// 		parts = append(parts, &genai.Part{
	// 			FileData: &genai.FileData{
	// 				FileURI:  genFile.URI,
	// 				MIMEType: genFile.MIMEType,
	// 			},
	// 		})
	// 	}
	// 	// donot use html elements such as sub sup
	// 	// Analyze the image text and pay special attention to precision. We need to return:
	// 	// 1. Proper complete image text with original transcript language (with accompanying illustration explanation in braces if exists). Ensure transcript precision and technical context is consistent and correct and also format it to have correct commas, quotes, parentheses and generally proper readable text, return only transcription, no timestamps. Additionally, For each highly unclear word provide placeholder of possible options and their possibility threshold in form of this example: "  [gone:0.7][done:0.5] " meaning that we are not sure about this word/phrase.
	// 	// 2. Concise subject name on original text content language only.
	// 	// 3. Markdown Format Summary in original content language: Start with brief introduction then specify key points, events, facts. You can generate a table for similar key facts.
	// 	// If image is empty or not understandable, please return only "Unknown note" for the name and keep other fields empty.
	// 	// 4. Quiz Object: Generate quiz with 10 questions and answers based on the image content and the language of the content. The quiz should be in JSON format. Each question should have a question, answer, and options. The 'options' should be in an array of strings containing four options. The 'answer' should be the correct answer from the options array.
	// 	// Return the result in JSON format.
	// 	if len(userTranscript) > 0 {
	// 		parts = append(parts, &genai.Part{
	// 			Text: userTranscript,
	// 		})
	// 	}
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	//Please use ONLY the following JSON schema for the response:
	// 	//Note = {'name': string, 'md_summary': string, 'transcription': string, 'quiz_json': Array<{'question': string, 'answer': string, 'options': Array<string>}>}
	// 	//Return: Note},
	// 	// result, err := client.Models.GenerateContent(ctx, "gemini-2.5-pro-preview-03-25", []*genai.Content{{Parts: parts}}, nil)

	// 	languagePrompt := `Detect the language of content and stick to it. Do not assume the language based on the first word or phrase, especially if it appears unusual or out of context. Analyze the first six to seven sentences of each file to accurately detect the primary language of the text.`

	// 	if languageCode != "auto" {
	// 		languagePrompt = `Transctipion language should be in "` + languageCode + ` code". (ISO 639-1 code). The main transcription text language should be "` + languageCode + `".`
	// 	}
	// 	languagePrefix := `identified primary language`
	// 	if languageCode != "auto" {
	// 		languagePrefix = `"` + languageCode + ` code language"`
	// 	}
	// 	result, err := client.Models.GenerateContent(ctx, modelName.String(), []*genai.Content{{Parts: parts}}, &genai.GenerateContentConfig{
	// 		ResponseMIMEType: "application/json",
	// 		CandidateCount:   1,
	// 		MaxOutputTokens:  50000,
	// 		Temperature:      floatPointer(0.8),
	// 		SystemInstruction: &genai.Content{
	// 			Parts: []*genai.Part{
	// 				{Text: `You are expert at transcribing and analyzing images, PDFs, audio files, and text content with the proper grammar. ` + languagePrompt + ` Follow the below instructions. Do not deviate from these requirements.  Return the response in JSON format with the specified fields.

	// 1. For audio content, transcribe it exactly in the ` + languagePrefix + `, ensuring absolute precision in nouns. Do not include timestamps. Use complete sentences with correct grammar and punctuation. For multiple audio files, transcribe these files preserving the chronological order as much as possible only based on the timestamp in their filenames or part numbers.
	// 2. For images and PDFs, transcribe the text while preserving the readable layout and pages order as closely as possible. In case of provided exam/test questions - do not solve them no matter what, only transcribe them. For any illustrations, include a brief description in the  ` + languagePrefix + ` within the transcription.
	// 3. Return the following JSON fields and make sure "\n" is escaped properly on every fields value as "\\n":
	//    - **transcription**: A complete, accurate transcription of all material text in Markdown language, using correct grammar, punctuation, and formatting in the ` + languagePrefix + `. Add commas and quotes where needed for clarity. Focus on grammatical correctness and use sentence breaks to represent natural pauses. Use complete sentences and a readable layout with paragraphs and line breaks. Do not alter sentence structure. Use only plain symbols and Markdown for formulas or similar content. Do not use LaTeX, HTML, or illustrate graphics with symbol art or Markdown.
	//    - **name**: A concise subject name in the ` + languagePrefix + ` only.
	//    - **md_summary**: A summary in the ` + languagePrefix + ` using Markdown. Begin with a brief introduction, followed by key points, events, and facts. Include a table for related key facts. Use only these Markdown directives if needed: bold, table elements, heading1, heading2, heading3, line breaks, new lines, emphasis, italic, blockquotes, lists (non-nested), code blocks, inline code. Do not use LaTeX, HTML, or nested lists.
	//    - **language**: The i ` + languagePrefix + ` in ISO 639-1 format (2-letter code, e.g., "en" for English).

	// Ensure all instructions are followed precisely, with no omissions or alterations. The response must be in JSON format, and all fields must be populated correctly based on the provided material.`},
	// 			},
	// 		},
	// 		ResponseSchema: &genai.Schema{
	// 			Type: "object",
	// 			// The schema for the AI response.
	// 			Properties: map[string]*genai.Schema{
	// 				"name": {
	// 					Type: "string",
	// 				},
	// 				"md_summary": {
	// 					Type: "string",
	// 				},
	// 				"transcription": {
	// 					Type: "string",
	// 				},
	// 				"language": {
	// 					Type: "string",
	// 				},
	// 			},
	// 			Required: []string{"name", "md_summary", "transcription", "language"},
	// 		},
	// 		ThinkingConfig: &genai.ThinkingConfig{
	// 			IncludeThoughts: true,
	// 			// ThinkingBudget:  Int32Pointer()
	// 		},
	// 	})

	// 	// client.Models.Cou

	// 	if err != nil {
	// 		fmt.Println("Error in GenerateContent:", err)
	// 		return nil, fmt.Errorf("%v", err)
	// 	}
	// 	var inputTokenCount int32
	// 	var thoughtsTokenCount int32
	// 	var outputTokenCount int32
	// 	var totalTokenCount int32
	// 	if result.UsageMetadata != nil {
	// 		inputTokenCount = result.UsageMetadata.PromptTokenCount
	// 		thoughtsTokenCount = result.UsageMetadata.ThoughtsTokenCount
	// 		outputTokenCount = result.UsageMetadata.CandidatesTokenCount
	// 		totalTokenCount = result.UsageMetadata.TotalTokenCount
	// 		fmt.Println("Input token count:", inputTokenCount)
	// 		fmt.Println("Output token count:", outputTokenCount)
	// 		fmt.Println("Thoughts token count:", thoughtsTokenCount)
	// 		fmt.Println("Total token count:", totalTokenCount)
	// 	} else {
	// 		fmt.Println("UsageMetadata is nil!")
	// 	}
	// 	llmResponseText, err := GetFirstCandidateTextWithThoughts(result)
	// 	if err != nil {
	// 		fmt.Println("Error getting first candidate text: ", err)
	// 		fmt.Println(result.Candidates)
	// 		if result.PromptFeedback != nil {

	// 			fmt.Println(result.PromptFeedback.BlockReason)
	// 			fmt.Println(result.PromptFeedback.BlockReasonMessage)
	// 			fmt.Println(result.PromptFeedback.SafetyRatings)
	// 			return nil, fmt.Errorf("content violation: %s ", result.PromptFeedback.BlockReasonMessage)
	// 		}
	// 		return nil, fmt.Errorf("error getting first candidate text: %v", err)
	// 	}
	// 	return &LLMResponse{
	// 		Response:           llmResponseText.Text,
	// 		Thoughts:           llmResponseText.Thoughts,
	// 		InputTokenCount:    inputTokenCount,
	// 		ThoughtsTokenCount: thoughtsTokenCount,
	// 		OutputTokenCount:   outputTokenCount,
	// 		TotalTokenCount:    totalTokenCount,
	// 		IsTest:             false,
	// 	}, nil

}
