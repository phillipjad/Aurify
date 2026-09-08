import { describe, expect, it } from 'vite-plus/test'
import { render, screen } from '@testing-library/react'
import { Sparkles } from 'lucide-react'

import { EmptyState } from '@/components/empty-state'

describe('EmptyState', () => {
  it('renders the title and an icon', () => {
    const { container } = render(<EmptyState icon={Sparkles} title="No covers yet" />)
    expect(screen.getByText('No covers yet')).toBeInTheDocument()
    expect(container.querySelector('svg')).toBeInTheDocument()
  })

  it('renders the optional description', () => {
    render(<EmptyState icon={Sparkles} title="No covers yet" description="Generate one from a playlist." />)
    expect(screen.getByText('Generate one from a playlist.')).toBeInTheDocument()
  })

  it('renders an optional action', () => {
    render(<EmptyState icon={Sparkles} title="No covers yet" action={<button>Browse playlists</button>} />)
    expect(screen.getByRole('button', { name: 'Browse playlists' })).toBeInTheDocument()
  })
})
