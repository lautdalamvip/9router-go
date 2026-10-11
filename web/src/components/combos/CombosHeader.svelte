<script lang="ts">
  import Button from '../../lib/ui/Button.svelte'

  interface Props {
    onCreateClick: () => void
    onAutoFreeClick?: () => void
    isBuildingAutoFree?: boolean
    onAutoFamilyClick?: () => void
    isBuildingAutoFamily?: boolean
    selectedCount?: number
    deletableCount?: number
    onDeleteSelected?: () => void
    onDeleteAll?: () => void
  }

  let {
    onCreateClick,
    onAutoFreeClick,
    isBuildingAutoFree = false,
    onAutoFamilyClick,
    isBuildingAutoFamily = false,
    selectedCount = 0,
    deletableCount = 0,
    onDeleteSelected,
    onDeleteAll,
  }: Props = $props()
</script>

<div class="flex flex-col gap-4">
  <div class="min-w-0">
    <p class="text-sm text-text-muted">
      Group models under one name, then pick a strategy per combo:
    </p>
    <ul class="text-sm text-text-muted mt-2 flex flex-col gap-1">
      <li>
        <span class="font-medium text-text-main">Fallback</span> — tries models in order (next on failure)
      </li>
      <li>
        <span class="font-medium text-text-main">Round Robin</span> — rotates models across requests to spread load
      </li>
      <li>
        <span class="font-medium text-text-main">Fusion</span> — queries all models in parallel, then a judge
        synthesizes one answer. Best quality, but costs the most: every request bills all panel models + the judge
        (N+1 calls)
      </li>
    </ul>
  </div>

  <!-- Upstream parity: the header carries one control. Bulk actions live in the
       selection bar next to the combos, so a destructive button never sits
       permanently beside Create. -->
  <div
    class="flex flex-wrap items-center gap-2 border-t border-border pt-4"
    role="group"
    aria-label="Combo actions"
  >
    <Button icon="add" size="sm" onclick={onCreateClick} class="whitespace-nowrap">
      Create Combo
    </Button>
    <div class="flex gap-2">
      {#if onAutoFamilyClick}
        <Button
          icon="hub"
          onclick={onAutoFamilyClick}
          disabled={isBuildingAutoFamily}
          variant="outline"
          class="flex-1 whitespace-nowrap"
        >
          {isBuildingAutoFamily ? 'Grouping...' : 'Auto Group by Model'}
        </Button>
      {/if}
      {#if onAutoFreeClick}
        <Button
          icon="auto_awesome"
          onclick={onAutoFreeClick}
          disabled={isBuildingAutoFree}
          variant="secondary"
          class="flex-1 whitespace-nowrap"
        >
          {isBuildingAutoFree ? 'Building...' : 'Auto Free Tier'}
        </Button>
      {/if}
    </div>
    <div class="flex gap-2">
      {#if onDeleteSelected}
        <Button
          icon="delete"
          onclick={onDeleteSelected}
          disabled={selectedCount === 0}
          variant="danger"
          class="flex-1 whitespace-nowrap"
        >
          Delete Selected ({selectedCount})
        </Button>
      {/if}
      {#if onDeleteAll}
        <Button
          icon="delete_sweep"
          onclick={onDeleteAll}
          disabled={deletableCount === 0}
          variant="outline"
          class="flex-1 whitespace-nowrap"
        >
          Delete All ({deletableCount})
        </Button>
      {/if}
    </div>
  </div>
</div>
