import { createAndListNFT } from './index';
import { readFileSync } from 'fs';


const main = async () => {
    // Sample data for listing the NFT
    const name = "Rect";
    const description = "This is a Rect NFT description.";
    const price = "0.001"; // Price in Ether

    // Read the image file from the filesystem
    // const file = new File([readFileSync('./methods/assets/sample-nft.png')], "sample-nft.png");
    await createAndListNFT(name, description, price, "./methods/assets/sample-nft.png");
};

main().catch(console.error);