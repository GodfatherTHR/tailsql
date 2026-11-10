import type { CodeLanguage } from '../types';

const JAVA_KEYWORDS = new Set([
  'abstract', 'assert', 'boolean', 'break', 'byte', 'case', 'catch', 'char', 'class',
  'const', 'continue', 'default', 'do', 'double', 'else', 'enum', 'extends', 'final',
  'finally', 'float', 'for', 'goto', 'if', 'implements', 'import', 'instanceof', 'int',
  'interface', 'long', 'native', 'new', 'package', 'private', 'protected', 'public',
  'return', 'short', 'static', 'strictfp', 'super', 'switch', 'synchronized', 'this',
  'throw', 'throws', 'transient', 'try', 'var', 'void', 'volatile', 'while', 'true',
  'false', 'null', 'record', 'sealed', 'permits', 'yield',
]);

const SQL_KEYWORDS = new Set([
  'select', 'from', 'where', 'insert', 'into', 'values', 'update', 'set', 'delete',
  'create', 'table', 'alter', 'drop', 'index', 'view', 'join', 'left', 'right', 'inner',
  'outer', 'on', 'group', 'by', 'order', 'having', 'limit', 'offset', 'as', 'and', 'or',
  'not', 'null', 'primary', 'key', 'foreign', 'references', 'unique', 'default', 'show',
  'tables', 'columns', 'describe', 'explain', 'distinct', 'count', 'sum', 'avg', 'min',
  'max', 'case', 'when', 'then', 'else', 'end', 'exists', 'in', 'like', 'between',
  'union', 'all', 'schema', 'if', 'cascade', 'truncate',
]);

const JAVA_TOKEN =
  /(\/\/.*$|\/\*.*?\*\/|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|@[A-Za-z_]\w*|\b[A-Za-z_]\w*\b|\b\d[\d_]*(?:\.\d+)?\b)/g;

const XML_TOKEN = /(<!--.*?-->|<\/?[A-Za-z][\w:.-]*|\/?>|"[^"]*"|[A-Za-z_][\w:.-]*|=)/g;

const SQL_TOKEN =
  /(--.*$|\/\*.*?\*\/|'(?:''|[^'])*'|"(?:[^"]|"")*"|\b[A-Za-z_]\w*\b|\b\d+(?:\.\d+)?\b)/g;

const YAML_KEY = /^(\s*(?:-\s*)?)([\w.-]+)(:)(.*)$/;
const YAML_STRING = /("(?:\\.|[^"\\])*"|'(?:''|[^'])*')/g;

export function highlightLine(line: string, language: CodeLanguage): string {
  switch (language) {
    case 'java':
    case 'groovy':
      return highlightJava(line);
    case 'xml':
      return highlightXML(line);
    case 'properties':
      return highlightProperties(line);
    case 'yaml':
      return highlightYAML(line);
    case 'sql':
      return highlightSQL(line);
    default:
      return escapeHtml(line);
  }
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function wrap(className: string, value: string): string {
  if (!className) {
    return escapeHtml(value);
  }
  return `<span class="${className}">${escapeHtml(value)}</span>`;
}

function highlightJava(line: string): string {
  let result = '';
  let cursor = 0;
  for (const match of line.matchAll(JAVA_TOKEN)) {
    const index = match.index ?? 0;
    if (index > cursor) {
      result += escapeHtml(line.slice(cursor, index));
    }
    const token = match[0];
    let className = '';
    if (token.startsWith('//') || token.startsWith('/*')) {
      className = 'tok-comment';
    } else if (token.startsWith('"') || token.startsWith("'")) {
      className = 'tok-string';
    } else if (token.startsWith('@')) {
      className = 'tok-annotation';
    } else if (JAVA_KEYWORDS.has(token)) {
      className = 'tok-keyword';
    } else if (/^\d/.test(token)) {
      className = 'tok-number';
    }
    result += wrap(className, token);
    cursor = index + token.length;
  }
  result += escapeHtml(line.slice(cursor));
  return result;
}

function highlightXML(line: string): string {
  let result = '';
  let cursor = 0;
  let insideTag = false;
  for (const match of line.matchAll(XML_TOKEN)) {
    const index = match.index ?? 0;
    if (index > cursor) {
      result += escapeHtml(line.slice(cursor, index));
    }
    const token = match[0];
    let className = '';
    if (token.startsWith('<!--')) {
      className = 'tok-comment';
    } else if (token.startsWith('<')) {
      className = 'tok-tag';
      insideTag = true;
    } else if (token === '>' || token === '/>') {
      className = 'tok-tag';
      insideTag = false;
    } else if (token.startsWith('"')) {
      className = 'tok-string';
    } else if (token === '=') {
      className = 'tok-punct';
    } else if (insideTag) {
      className = 'tok-attr';
    }
    result += wrap(className, token);
    cursor = index + token.length;
  }
  result += escapeHtml(line.slice(cursor));
  return result;
}

function highlightProperties(line: string): string {
  const trimmed = line.trimStart();
  if (trimmed.startsWith('#') || trimmed.startsWith('!')) {
    return wrap('tok-comment', line);
  }
  let separator = -1;
  let escaped = false;
  for (let index = 0; index < line.length; index += 1) {
    const character = line[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (character === '\\') {
      escaped = true;
      continue;
    }
    if (character === '=' || character === ':') {
      separator = index;
      break;
    }
  }
  if (separator < 0) {
    return escapeHtml(line);
  }
  return (
    wrap('tok-key', line.slice(0, separator)) +
    wrap('tok-punct', line[separator]) +
    wrap('tok-string', line.slice(separator + 1))
  );
}

function highlightYAML(line: string): string {
  const trimmed = line.trimStart();
  if (trimmed.startsWith('#')) {
    return wrap('tok-comment', line);
  }
  const match = line.match(YAML_KEY);
  if (!match) {
    return highlightYAMLStrings(line);
  }
  return (
    escapeHtml(match[1]) +
    wrap('tok-key', match[2]) +
    wrap('tok-punct', match[3]) +
    highlightYAMLStrings(match[4])
  );
}

function highlightYAMLStrings(value: string): string {
  let result = '';
  let cursor = 0;
  for (const match of value.matchAll(YAML_STRING)) {
    const index = match.index ?? 0;
    if (index > cursor) {
      result += escapeHtml(value.slice(cursor, index));
    }
    result += wrap('tok-string', match[0]);
    cursor = index + match[0].length;
  }
  result += escapeHtml(value.slice(cursor));
  return result;
}

function highlightSQL(line: string): string {
  let result = '';
  let cursor = 0;
  for (const match of line.matchAll(SQL_TOKEN)) {
    const index = match.index ?? 0;
    if (index > cursor) {
      result += escapeHtml(line.slice(cursor, index));
    }
    const token = match[0];
    let className = '';
    if (token.startsWith('--') || token.startsWith('/*')) {
      className = 'tok-comment';
    } else if (token.startsWith("'") || token.startsWith('"')) {
      className = 'tok-string';
    } else if (SQL_KEYWORDS.has(token.toLowerCase())) {
      className = 'tok-keyword';
    } else if (/^\d/.test(token)) {
      className = 'tok-number';
    }
    result += wrap(className, token);
    cursor = index + token.length;
  }
  result += escapeHtml(line.slice(cursor));
  return result;
}
