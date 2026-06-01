import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { Button } from '@/components/ui/button'

describe('Button', () => {
  it('renders its children', () => {
    render(<Button>Click me</Button>)
    expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument()
  })

  it('calls onClick when pressed', async () => {
    const onClick = vi.fn()
    render(<Button onClick={onClick}>Go</Button>)

    await userEvent.click(screen.getByRole('button', { name: 'Go' }))

    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('does not fire onClick when disabled', async () => {
    const onClick = vi.fn()
    render(
      <Button disabled onClick={onClick}>
        Nope
      </Button>,
    )

    await userEvent.click(screen.getByRole('button', { name: 'Nope' }))

    expect(onClick).not.toHaveBeenCalled()
  })

  it('applies the requested variant and size classes', () => {
    render(
      <Button variant="outline" size="icon">
        Icon
      </Button>,
    )
    const button = screen.getByRole('button', { name: 'Icon' })
    expect(button).toHaveClass('border') // outline variant
    expect(button).toHaveClass('h-10', 'w-10') // icon size
  })

  it('forwards native button attributes', () => {
    render(<Button type="submit">Submit</Button>)
    expect(screen.getByRole('button', { name: 'Submit' })).toHaveAttribute('type', 'submit')
  })
})
