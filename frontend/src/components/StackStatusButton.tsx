import { OPEN_STACK_STATUS_EVENT } from '../stackStatusEvents'

export default function StackStatusButton({ className = '' }: { className?: string }) {
  return (
    <button
      type="button"
      onClick={() => window.dispatchEvent(new CustomEvent(OPEN_STACK_STATUS_EVENT))}
      className={`rounded border border-white/10 bg-white/[0.06] px-2.5 py-1.5 text-[11px] font-black uppercase tracking-wide text-zinc-300 transition hover:border-violet-300/40 hover:bg-white/10 hover:text-white ${className}`}
    >
      Stack status
    </button>
  )
}
