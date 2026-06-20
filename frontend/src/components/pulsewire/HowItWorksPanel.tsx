export default function HowItWorksPanel() {
  return (
    <div className="rounded-xl border border-[#2A2A2E] bg-[#0E0E11] p-4">
      <h3 className="mb-2 text-[13px] font-bold text-[#A970FF]">How it works</h3>
      <p className="text-xs leading-relaxed text-[#ADADB8]">
        Pulse detects the Twitch moment first, then tracks where it spreads. Scores are descriptive,
        not editorial.
      </p>
      <a
        href="https://github.com/Aron-Chu/streamclone/blob/master/docs/options.md#pulse-wire-story-graph"
        target="_blank"
        rel="noreferrer"
        className="mt-2 inline-block text-xs font-semibold text-[#A970FF] hover:text-[#C9A7FF]"
      >
        Learn more →
      </a>
    </div>
  )
}
