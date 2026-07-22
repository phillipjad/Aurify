import { DIMENSIONS } from './dimensions'

// Four archetypal weight distributions across the seven dimensions, purely to
// preview the palette step of the pipeline on the landing page. Real cover
// art still comes from the imagegen sidecar, which is a SCAFFOLD (see
// backend/internal/platform/llm/imagegen/client.go). Showing the palette
// itself is the honest "example output" until that's wired up.
const EXAMPLES: { name: string; weights: Record<string, number> }[] = [
  { name: 'Late-night drive', weights: { energetic: 0.45, introspective: 0.3, melancholic: 0.25 } },
  { name: 'Sunday acoustic', weights: { organic: 0.5, intimate: 0.3, introspective: 0.2 } },
  { name: 'Breakup on repeat', weights: { melancholic: 0.5, introspective: 0.3, intimate: 0.2 } },
  { name: 'Festival warmup', weights: { energetic: 0.4, danceable: 0.35, euphoric: 0.25 } },
]

const HEX_BY_NAME = new Map(DIMENSIONS.map((d) => [d.name, d.hex]))

/** A conic-gradient wheel of the given dimensions, proportional to weight. */
function paletteGradient(weights: Record<string, number>): string {
  let angle = 0
  const stops = Object.entries(weights).map(([name, weight]) => {
    const start = angle
    angle += weight * 360
    return `${HEX_BY_NAME.get(name)} ${start}deg ${angle}deg`
  })
  return `conic-gradient(${stops.join(', ')})`
}

export function ExampleCovers() {
  return (
    <ul className="grid grid-cols-2 gap-4 sm:grid-cols-4">
      {EXAMPLES.map((example) => (
        <li key={example.name} className="space-y-2">
          <div
            aria-hidden="true"
            className="aspect-square rounded-xl ring-1 ring-border"
            style={{ backgroundImage: paletteGradient(example.weights) }}
          />
          <p className="text-center text-sm font-medium text-pretty">{example.name}</p>
        </li>
      ))}
    </ul>
  )
}
