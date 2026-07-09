import { useEffect, useRef, useState } from 'react';
import { useLocation } from 'react-router';
import { ApiError } from '../api/client';
import { useChatHistory, useClearChat, useSendChat } from '../api/hooks';
import { MonoLabel } from '../ui/kit';

const CHIPS = ['Why easy today?', 'Am I improving?', 'Running × CrossFit?'];

type BubbleVM = { role: 'user' | 'assistant' | 'context'; text: string };

function Bubble({ b }: { b: BubbleVM }) {
  const coach = b.role !== 'user';
  return (
    <div
      style={{
        maxWidth: '85%',
        alignSelf: coach ? 'flex-start' : 'flex-end',
        background: coach ? 'var(--surface)' : 'var(--green-tint-2)',
        border: `1px solid ${coach ? 'var(--surface-border)' : 'var(--green-border)'}`,
        borderRadius: 16,
        padding: '11px 14px',
        fontSize: 13.5,
        lineHeight: 1.55,
        color: coach ? '#D6DDE3' : '#DCEFE3',
        whiteSpace: 'pre-wrap',
      }}
    >
      {b.role === 'context' && (
        <div
          style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 9,
            letterSpacing: '.16em',
            color: 'var(--label)',
            marginBottom: 6,
          }}
        >
          THIS MORNING
        </div>
      )}
      {b.text}
    </div>
  );
}

export function CoachPage() {
  const location = useLocation();
  const prefill = (location.state as { prefill?: string } | null)?.prefill;
  const { data: history, isLoading } = useChatHistory();
  const send = useSendChat();
  const clear = useClearChat();
  const [draft, setDraft] = useState('');
  const scrollRef = useRef<HTMLDivElement>(null);

  const bubbles: BubbleVM[] = [
    ...(prefill ? [{ role: 'context' as const, text: prefill }] : []),
    ...(history ?? []).map((m) => ({
      role: (m.role === 'user' ? 'user' : 'assistant') as 'user' | 'assistant',
      text: m.content,
    })),
    ...(send.isPending && send.variables ? [{ role: 'user' as const, text: send.variables }] : []),
  ];

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
  }, [history?.length, send.isPending]);

  const submit = (text: string) => {
    const message = text.trim();
    if (!message || send.isPending) return;
    setDraft('');
    send.mutate(message);
  };

  const sendError =
    send.error instanceof ApiError
      ? send.error.message
      : send.error
        ? 'Something went wrong.'
        : null;

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
      <div
        style={{
          flex: 'none',
          padding: '6px 18px 10px',
          borderBottom: '1px solid var(--hairline)',
          display: 'flex',
          alignItems: 'center',
        }}
      >
        <MonoLabel>{'// COACH · ASK YOUR DATA'}</MonoLabel>
        {(history?.length ?? 0) > 0 && (
          <button
            onClick={() => {
              if (window.confirm('Clear the whole conversation?')) clear.mutate();
            }}
            style={{
              marginLeft: 'auto',
              background: 'none',
              border: 'none',
              fontFamily: 'var(--font-mono)',
              fontSize: 10,
              letterSpacing: '.12em',
              color: 'var(--faint)',
            }}
          >
            CLEAR
          </button>
        )}
      </div>

      <div
        ref={scrollRef}
        className="scroll-pane"
        style={{
          flex: 1,
          minHeight: 0,
          padding: '16px 16px 8px',
          display: 'flex',
          flexDirection: 'column',
          gap: 12,
          maxWidth: 680,
          width: '100%',
          margin: '0 auto',
        }}
      >
        {isLoading && (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 30 }}>
            <div className="spinner" role="status" aria-label="loading" />
          </div>
        )}
        {!isLoading && bubbles.length === 0 && (
          <div style={{ color: 'var(--muted)', fontSize: 14, lineHeight: 1.6, padding: '30px 8px' }}>
            Ask anything about your own data — readiness, pace trends, how running is feeding your
            CrossFit. The coach answers from your Garmin history.
          </div>
        )}
        {bubbles.map((b, i) => (
          <Bubble key={i} b={b} />
        ))}
        {send.isPending && (
          <div
            style={{
              alignSelf: 'flex-start',
              display: 'flex',
              gap: 5,
              padding: '12px 14px',
            }}
            aria-label="coach is thinking"
          >
            <span className="pulse-dot" style={{ background: 'var(--green)' }} />
            <span className="pulse-dot" style={{ background: 'var(--green)', animationDelay: '.3s' }} />
            <span className="pulse-dot" style={{ background: 'var(--green)', animationDelay: '.6s' }} />
          </div>
        )}
        {sendError && (
          <div
            style={{
              alignSelf: 'flex-start',
              maxWidth: '85%',
              background: 'var(--red-tint)',
              border: '1px solid var(--red-border)',
              borderRadius: 16,
              padding: '11px 14px',
              fontSize: 13,
              color: 'var(--red)',
            }}
          >
            {sendError}{' '}
            <button
              onClick={() => send.variables && send.mutate(send.variables)}
              style={{
                background: 'none',
                border: 'none',
                color: 'var(--red)',
                textDecoration: 'underline',
                fontSize: 13,
                padding: 0,
              }}
            >
              retry
            </button>
          </div>
        )}
      </div>

      <div
        style={{
          flex: 'none',
          padding: '8px 14px 12px',
          borderTop: '1px solid var(--hairline)',
          maxWidth: 680,
          width: '100%',
          margin: '0 auto',
        }}
      >
        <div className="scroll-pane" style={{ display: 'flex', gap: 7, overflowX: 'auto', paddingBottom: 9 }}>
          {CHIPS.map((c) => (
            <button key={c} className="chip" style={{ flex: 'none' }} onClick={() => submit(c)}>
              {c}
            </button>
          ))}
        </div>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            submit(draft);
          }}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            background: 'var(--subtle)',
            border: '1px solid var(--inset-border)',
            borderRadius: 22,
            padding: '6px 6px 6px 16px',
          }}
        >
          <input
            aria-label="Ask about your training"
            placeholder="Ask about your training…"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            style={{
              flex: 1,
              background: 'none',
              border: 'none',
              outline: 'none',
              fontSize: 13.5,
              color: 'var(--text)',
              padding: '8px 0',
            }}
          />
          <button
            type="submit"
            aria-label="Send"
            disabled={send.isPending || !draft.trim()}
            style={{
              width: 34,
              height: 34,
              flex: 'none',
              borderRadius: '50%',
              border: 'none',
              background: send.isPending || !draft.trim() ? '#1a212a' : 'var(--green)',
              color: send.isPending || !draft.trim() ? 'var(--faint)' : 'var(--on-green)',
              fontSize: 15,
            }}
          >
            ↑
          </button>
        </form>
      </div>
    </div>
  );
}
