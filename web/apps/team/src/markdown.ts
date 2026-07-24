import { Marked, Renderer, type Tokens } from 'marked';

function escapeHTML(value: string) {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function safeLinkTarget(value: string) {
  const href = value.trim();
  if (/^https?:\/\//i.test(href) || /^mailto:/i.test(href) || href.startsWith('#')) {
    return href;
  }
  return '';
}

const renderer = new Renderer();

renderer.html = ({ text }: Tokens.HTML | Tokens.Tag) => escapeHTML(text);

renderer.link = function ({ href, title, tokens }: Tokens.Link) {
  const label = this.parser.parseInline(tokens);
  const target = safeLinkTarget(href);
  if (!target) return label;
  const titleAttribute = title ? ` title="${escapeHTML(title)}"` : '';
  return `<a href="${escapeHTML(target)}"${titleAttribute} target="_blank" rel="noopener noreferrer nofollow">${label}</a>`;
};

renderer.image = ({ text }: Tokens.Image) => escapeHTML(text);

const safeMarkdown = new Marked({
  breaks: true,
  gfm: true,
  renderer,
});

export function renderSafeMarkdown(content: string) {
  return safeMarkdown.parse(content || '') as string;
}
