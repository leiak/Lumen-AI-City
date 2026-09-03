/**
 * 旁路流过滤（§E.4）
 *
 * 用于在 LLM/第三方服务调用前过滤敏感信息：
 * - 手机号、身份证、地址
 * - 用户名密码 token
 * - 第三方业务隐私
 */

const SENSITIVE_PATTERNS = [
  { name: 'phone_cn', regex: /1[3-9]\d{9}/g, mask: '1XX-XXXX-XXXX' },
  { name: 'id_card_cn', regex: /\d{17}[\dXx]/g, mask: 'XXXXXXXXXXXXXXXXXX' },
  { name: 'email', regex: /[\w.-]+@[\w.-]+\.\w+/g, mask: 'X@X.com' },
  { name: 'jwt', regex: /eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g, mask: 'JWT_TOKEN' },
  { name: 'api_key', regex: /sk-[A-Za-z0-9_-]{20,}/g, mask: 'sk-XXXX' },
];

export interface FilterResult {
  clean: string;
  redacted: Record<string, number>;
}

export function bypassFilter(text: string): FilterResult {
  let clean = text;
  const redacted: Record<string, number> = {};

  for (const pattern of SENSITIVE_PATTERNS) {
    const matches = clean.match(pattern.regex);
    if (matches) {
      redacted[pattern.name] = matches.length;
      clean = clean.replace(pattern.regex, pattern.mask);
    }
  }

  return { clean, redacted };
}
