import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vite-plus/test'

import { DIMENSIONS } from './dimensions'
import { ExampleCovers } from './example-covers'

describe('ExampleCovers', () => {
  // The examples name dimensions as strings and look their colors up in a map,
  // so an example naming a dimension DIMENSIONS does not carry emits a stop
  // reading `undefined 234deg 306deg`. That is not one missing wedge: the whole
  // conic-gradient fails to parse and the swatch renders blank. No type check
  // sees it, because the lookup is a string key.
  it('every example weight names a known dimension', () => {
    const { container } = render(<ExampleCovers />)
    const known = new Set(DIMENSIONS.map((d) => d.name))

    const swatches = container.querySelectorAll<HTMLElement>('[aria-hidden="true"]')
    expect(swatches).toHaveLength(4)
    for (const swatch of swatches) {
      expect(swatch.style.backgroundImage).not.toContain('undefined')
      expect(swatch.style.backgroundImage).toMatch(/^conic-gradient\(/)
    }

    // And the reverse: the whole point of adding a dimension is that it shows.
    // jsdom resolves the hex, so #31C3B3 arrives as rgb, over the 72 degrees a
    // weight of 0.2 is owed.
    expect(known).toContain('driving')
    const gradients = [...swatches].map((s) => s.style.backgroundImage).join(' ')
    expect(gradients).toContain('rgb(49, 195, 179) 234deg 306deg')
  })

  it('labels each example', () => {
    render(<ExampleCovers />)
    expect(screen.getByText('Festival warmup')).toBeInTheDocument()
  })
})
