package models

type NoteUrlRequstIn struct {
	NoteId   uint   `json:"note_id"`
	FileName string `json:"file_name"`
}

type NoteFilesUploadRequestIn struct {
	Notes []NoteUrlRequstIn `json:"notes"`
}

type NoteFileUploadRequestOut struct {
	NoteId    uint   `json:"note_id"`
	FileName  string `json:"file_name"`
	UploadUrl string `json:"upload_url"`
}

type NoteFilesUploadRequestOut struct {
	Notes []NoteFileUploadRequestOut `json:"notes"`
}
