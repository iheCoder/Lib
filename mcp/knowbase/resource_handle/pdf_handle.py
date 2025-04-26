import fitz  # PyMuPDF

class HandleMode:
    BY_LINES = 0
    BY_CHARS = 1

def extract_pdf_text_into_chunks(filepath, size_per_chunk, mode=HandleMode.BY_LINES):
    text = extract_text_from_pdf(filepath)

    if mode == HandleMode.BY_LINES:
        return split_text_into_chunks_by_lines(text, size_per_chunk)
    elif mode == HandleMode.BY_CHARS:
        return split_text_into_chunks_by_chars(text, size_per_chunk)
    else:
        return split_text_into_chunks_by_lines(text, size_per_chunk)

def extract_text_from_pdf(filepath):
    doc = fitz.open(filepath)
    full_text = ""

    for page in doc:
        full_text += page.get_text()
        full_text += "\n"  # 每页结束加换行，避免连在一起
    return full_text

def split_text_into_chunks_by_lines(text, lines_per_chunk):
    lines = text.split('\n')
    chunks = []
    for i in range(0, len(lines), lines_per_chunk):
        chunk = '\n'.join(lines[i:i+lines_per_chunk])
        chunks.append(chunk)
    return chunks

def split_text_into_chunks_by_chars(text, chars_per_chunk):
    chunks = []
    for i in range(0, len(text), chars_per_chunk):
        chunk = text[i:i+chars_per_chunk]
        chunks.append(chunk)
    return chunks

if __name__ == "__main__":
    filepath = "../testdata/redbook-5th-edition.pdf"

    # 按字符切片
    chunks = extract_pdf_text_into_chunks(filepath, 1000, HandleMode.BY_CHARS)

    for idx, chunk in enumerate(chunks):
        print(f"Chunk {idx+1}:\n{chunk}\n{'='*40}")