<script lang="ts">
    const url = "http://localhost:8080/";

    let number: number = $state(0);
    let responseNumber: number = $state(0);

    type Request = {
        number: number;
    };

    type Response = {
        number: number;
    };

    async function handleClick() {
        try {
            const body: Request = { number };
            const response = await fetch(url, {
                method: "POST",
                headers: {
                    "Content-Type": "application/json",
                },
                body: JSON.stringify(body),
            });
            const data: Response = await response.json();
            console.log(data);
            responseNumber = data.number;
        } catch (err) {
            console.error(err);
        }
    }
</script>

<div class="flex flex-col justify-center items-center h-screen gap-4">
    <input type="number" bind:value={number} />
    <p>Response: {responseNumber}</p>

    <button class="bg-blue-500 text-white px-4 py-2 rounded" onclick={handleClick}>Send to server</button>
</div>
