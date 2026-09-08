import { describe, expect, it } from 'vite-plus/test'
import { fireEvent, render, screen } from '@testing-library/react'

import { ImageWithFallback } from '@/components/image-with-fallback'

describe('ImageWithFallback', () => {
  it('renders the image when a src is given', () => {
    render(<ImageWithFallback src="http://x/a.png" alt="art" fallback={<span>placeholder</span>} />)
    expect(screen.getByRole('img', { name: 'art' })).toHaveAttribute('src', 'http://x/a.png')
    expect(screen.queryByText('placeholder')).not.toBeInTheDocument()
  })

  it('renders the fallback when there is no src', () => {
    render(<ImageWithFallback alt="art" fallback={<span>placeholder</span>} />)
    expect(screen.getByText('placeholder')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('swaps to the fallback when the image fails to load (no broken-image glyph)', () => {
    render(<ImageWithFallback src="http://x/broken.png" alt="art" fallback={<span>placeholder</span>} />)
    fireEvent.error(screen.getByRole('img'))
    expect(screen.getByText('placeholder')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })
})
