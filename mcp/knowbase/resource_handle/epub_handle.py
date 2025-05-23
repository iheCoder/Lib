import ebooklib
from ebooklib import epub
from bs4 import BeautifulSoup
import sys
import json

class HandleMode:
    BY_LINES = 0
    BY_CHARS = 1
    BY_PARAGRAPH_SMART = 2  # 新增：按段落智能分块

def extract_text_from_epub(filepath):
    book = epub.read_epub(filepath)
    full_text = ""

    for item in book.get_items():
        if item.get_type() == ebooklib.ITEM_DOCUMENT:
            # Parse HTML content
            soup = BeautifulSoup(item.get_content(), 'html.parser')
            # Extract text, remove HTML tags
            content = soup.get_text()
            full_text += content
            full_text += "\n"  # Add line break between sections

    return full_text

def extract_paragraphs_from_epub(filepath):
    book = epub.read_epub(filepath)
    paragraphs = []
    for item in book.get_items():
        if item.get_type() == ebooklib.ITEM_DOCUMENT:
            soup = BeautifulSoup(item.get_content(), 'html.parser')
            for p in soup.find_all('p'):
                text = p.get_text().strip()
                if text:
                    paragraphs.append(text)
    return paragraphs

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

def split_paragraphs_smart(paragraphs, max_chars_per_chunk):
    chunks = []
    current_chunk = ""
    for para in paragraphs:
        if len(current_chunk) + len(para) + 1 <= max_chars_per_chunk:
            if current_chunk:
                current_chunk += "\n" + para
            else:
                current_chunk = para
        else:
            if current_chunk:
                chunks.append(current_chunk)
            current_chunk = para
    if current_chunk:
        chunks.append(current_chunk)
    return chunks

def extract_epub_text_into_chunks(filepath, size_per_chunk, mode=HandleMode.BY_LINES):
    if mode == HandleMode.BY_PARAGRAPH_SMART:
        paragraphs = extract_paragraphs_from_epub(filepath)
        return split_paragraphs_smart(paragraphs, size_per_chunk)
    text = extract_text_from_epub(filepath)

    if mode == HandleMode.BY_LINES:
        return split_text_into_chunks_by_lines(text, size_per_chunk)
    elif mode == HandleMode.BY_CHARS:
        return split_text_into_chunks_by_chars(text, size_per_chunk)
    else:
        return split_text_into_chunks_by_lines(text, size_per_chunk)

if __name__ == "__main__":
    if len(sys.argv) < 4:
        print("Usage: python epub_handle.py <epub_path> <size_per_chunk> <mode>")
        sys.exit(1)

    epub_path = sys.argv[1]
    size_per_chunk = int(sys.argv[2])
    mode = int(sys.argv[3])

    chunks = extract_epub_text_into_chunks(epub_path, size_per_chunk, mode)

    print(json.dumps(chunks, ensure_ascii=False))
