import assert from 'node:assert/strict';
import { renderSafeMarkdown } from '../.test-dist/src/markdown.js';

const malicious = renderSafeMarkdown(`
# 安全标题

<script>alert('xss')</script>
<img src=x onerror="alert(1)">
[危险链接](javascript:alert(1))
![危险图片](javascript:alert(1))
[相对链接](/api/state)
`);

assert.doesNotMatch(malicious, /<script|<img|href="javascript:|src="javascript:/i);
assert.match(malicious, /&lt;script&gt;/);
assert.match(malicious, /&lt;img src=x onerror=/);
assert.doesNotMatch(malicious, /href="\/api\/state"/);

const safe = renderSafeMarkdown(`
**重要结论**

- [文档](https://example.com/docs?q=team)
- [邮件](mailto:team@example.com)
- \`inline code\`
`);

assert.match(safe, /<strong>重要结论<\/strong>/);
assert.match(safe, /href="https:\/\/example\.com\/docs\?q=team"/);
assert.match(safe, /target="_blank"/);
assert.match(safe, /rel="noopener noreferrer nofollow"/);
assert.match(safe, /href="mailto:team@example\.com"/);

console.log('team markdown security tests passed');
