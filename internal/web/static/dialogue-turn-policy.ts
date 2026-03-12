const FINAL_PUNCTUATION_RE = /[.!?][)"'\]]*$/;
const CONTINUATION_PUNCTUATION_RE = /(?:,|:|;|-|--|—|…|\.\.\.)[)"'\]]*$/;

const HESITATION_TOKENS = new Set([
  'ah', 'eh', 'er', 'erm', 'hmm', 'hm', 'mm', 'mmm', 'uh', 'uhh', 'uhm', 'um', 'umm',
  'well', 'like',
]);

const BACKCHANNEL_PHRASES = new Set([
  'got it',
  'i see',
  'makes sense',
  'mm-hmm',
  'mmhmm',
  'ok',
  'okay',
  'right',
  'sure',
  'thanks',
  'thank you',
  'yeah',
  'yep',
  'yes',
]);

const COMPLETE_SHORT_UTTERANCES = new Set([
  'go on',
  'hold on',
  'nevermind',
  'never mind',
  'no',
  'not now',
  'please continue',
  'repeat that',
  'resume',
  'start over',
  'stop',
  'wait',
  'yes',
]);

const TRAILING_CONTINUATION_TOKENS = new Set([
  'a', 'an', 'and', 'around', 'as', 'at', 'because', 'but', 'for', 'from',
  'if', 'in', 'into', 'like', 'my', 'of', 'on', 'or', 'so', 'that',
  'the', 'then', 'this', 'to', 'under', 'until', 'when', 'while', 'with', 'your',
]);

const LEADING_QUESTION_TOKENS = new Set([
  'are', 'can', 'could', 'did', 'do', 'does', 'how', 'is', 'should',
  'what', 'when', 'where', 'who', 'why', 'will', 'would',
]);

const CONTINUATION_WAIT_MS = 900;
const SHORT_CONTINUATION_WAIT_MS = 650;

export type DialogueTurnAction = 'finalize' | 'continue' | 'backchannel';

export type DialogueTurnDecision = {
  action: DialogueTurnAction;
  combinedText: string;
  currentText: string;
  reason: string;
  waitMs: number;
};

export type DialogueTurnSegment = {
  priorText?: string;
  text?: string;
  durationMs?: number;
  interruptedAssistant?: boolean;
};

type DialogueTurnCallbacks = {
  onFinalize?: (text: string, decision: DialogueTurnDecision) => void;
  onContinue?: (decision: DialogueTurnDecision) => void;
  onBackchannel?: (decision: DialogueTurnDecision) => void;
};

function normalizeText(value: unknown): string {
  return String(value || '').replace(/\s+/g, ' ').trim();
}

function tokenize(text: string): string[] {
  return normalizeText(text)
    .toLowerCase()
    .replace(/[^a-z0-9' -]+/gi, ' ')
    .split(/\s+/)
    .filter(Boolean);
}

function lastToken(text: string): string {
  const tokens = tokenize(text);
  return tokens.length > 0 ? tokens[tokens.length - 1] : '';
}

function hasUnbalancedClosers(text: string): boolean {
  const pairs: Record<string, string> = { '(': ')', '[': ']', '{': '}', '"': '"', "'": "'" };
  const counts = Object.keys(pairs).reduce<Record<string, number>>((acc, opener) => {
    acc[opener] = 0;
    acc[pairs[opener]] = 0;
    return acc;
  }, {});
  for (const ch of text) {
    if (counts[ch] !== undefined) counts[ch] += 1;
  }
  return counts['('] > counts[')']
    || counts['['] > counts[']']
    || counts['{'] > counts['}']
    || counts['"'] % 2 === 1
    || counts["'"] % 2 === 1;
}

function isHesitationOnly(text: string): boolean {
  const tokens = tokenize(text);
  return tokens.length > 0 && tokens.every((token) => HESITATION_TOKENS.has(token));
}

function isBackchannel(text: string): boolean {
  const normalized = normalizeText(text).toLowerCase();
  if (!normalized) return false;
  if (BACKCHANNEL_PHRASES.has(normalized)) return true;
  return isHesitationOnly(normalized);
}

function looksLikeCompleteQuestion(text: string, tokens: string[]): boolean {
  if (FINAL_PUNCTUATION_RE.test(text)) return true;
  if (tokens.length < 3) return false;
  return LEADING_QUESTION_TOKENS.has(tokens[0]);
}

function looksLikeCompleteShortUtterance(text: string): boolean {
  return COMPLETE_SHORT_UTTERANCES.has(normalizeText(text).toLowerCase());
}

function looksIncomplete(text: string, currentText: string, durationMs: number, tokens: string[]): string {
  if (!text) return 'empty';
  if (CONTINUATION_PUNCTUATION_RE.test(text)) return 'continuation_punctuation';
  if (hasUnbalancedClosers(text)) return 'open_phrase';
  if (isHesitationOnly(currentText)) return 'hesitation';

  const tail = lastToken(text);
  if (tail && TRAILING_CONTINUATION_TOKENS.has(tail)) {
    return 'trailing_connector';
  }

  if (tokens.length <= 2 && !looksLikeCompleteShortUtterance(text)) {
    return 'too_short';
  }

  if (!FINAL_PUNCTUATION_RE.test(text) && !looksLikeCompleteQuestion(text, tokens)) {
    if (durationMs < 900 && tokens.length <= 6) {
      return 'short_unpunctuated';
    }
    if (currentText.length < 18 && tokens.length <= 4) {
      return 'fragment';
    }
  }

  return '';
}

export function classifyDialogueTurnSegment(segment: DialogueTurnSegment = {}): DialogueTurnDecision {
  const priorText = normalizeText(segment.priorText);
  const currentText = normalizeText(segment.text);
  const combinedText = normalizeText([priorText, currentText].filter(Boolean).join(' '));
  const durationMs = Math.max(0, Number(segment.durationMs) || 0);
  const tokens = tokenize(combinedText);

  if (!combinedText) {
    return {
      action: 'backchannel',
      combinedText: '',
      currentText: '',
      reason: 'empty',
      waitMs: SHORT_CONTINUATION_WAIT_MS,
    };
  }

  if (!priorText && isBackchannel(currentText) && segment.interruptedAssistant) {
    return {
      action: 'backchannel',
      combinedText,
      currentText,
      reason: 'assistant_backchannel',
      waitMs: SHORT_CONTINUATION_WAIT_MS,
    };
  }

  const incompleteReason = looksIncomplete(combinedText, currentText, durationMs, tokens);
  if (incompleteReason) {
    return {
      action: 'continue',
      combinedText,
      currentText,
      reason: incompleteReason,
      waitMs: tokens.length <= 2 ? SHORT_CONTINUATION_WAIT_MS : CONTINUATION_WAIT_MS,
    };
  }

  return {
    action: 'finalize',
    combinedText,
    currentText,
    reason: FINAL_PUNCTUATION_RE.test(combinedText) ? 'terminal_punctuation' : 'semantic_completion',
    waitMs: 0,
  };
}

export class DialogueTurnController {
  _pendingText: string;
  _timer: number | null;
  _callbacks: DialogueTurnCallbacks;

  constructor(callbacks: DialogueTurnCallbacks = {}) {
    this._pendingText = '';
    this._timer = null;
    this._callbacks = callbacks;
  }

  get pendingText(): string {
    return this._pendingText;
  }

  reset() {
    if (this._timer !== null) {
      window.clearTimeout(this._timer);
      this._timer = null;
    }
    this._pendingText = '';
  }

  consume(segment: DialogueTurnSegment = {}): DialogueTurnDecision {
    const decision = classifyDialogueTurnSegment({
      ...segment,
      priorText: this._pendingText || segment.priorText || '',
    });

    if (decision.action === 'continue') {
      this._pendingText = decision.combinedText;
      this._scheduleFinalize(decision.waitMs);
      if (typeof this._callbacks.onContinue === 'function') {
        this._callbacks.onContinue(decision);
      }
      return decision;
    }

    if (decision.action === 'backchannel') {
      if (typeof this._callbacks.onBackchannel === 'function') {
        this._callbacks.onBackchannel(decision);
      }
      return decision;
    }

    this._finalize(decision);
    return decision;
  }

  flush(reason = 'timeout') {
    const text = normalizeText(this._pendingText);
    if (!text) {
      this.reset();
      return null;
    }
    const decision: DialogueTurnDecision = {
      action: 'finalize',
      combinedText: text,
      currentText: text,
      reason,
      waitMs: 0,
    };
    this._finalize(decision);
    return decision;
  }

  _scheduleFinalize(waitMs: number) {
    if (this._timer !== null) {
      window.clearTimeout(this._timer);
      this._timer = null;
    }
    this._timer = window.setTimeout(() => {
      this._timer = null;
      this.flush('continuation_timeout');
    }, Math.max(0, Number(waitMs) || CONTINUATION_WAIT_MS));
  }

  _finalize(decision: DialogueTurnDecision) {
    if (this._timer !== null) {
      window.clearTimeout(this._timer);
      this._timer = null;
    }
    const text = normalizeText(decision.combinedText);
    this._pendingText = '';
    if (!text) return;
    if (typeof this._callbacks.onFinalize === 'function') {
      this._callbacks.onFinalize(text, {
        ...decision,
        combinedText: text,
        currentText: normalizeText(decision.currentText),
      });
    }
  }
}
