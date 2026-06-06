<script lang="ts">
    let { children } = $props();

    import "../app.css";
</script>

<svelte:head>
    <title>Robot Control</title>
</svelte:head>

{@render children()}
