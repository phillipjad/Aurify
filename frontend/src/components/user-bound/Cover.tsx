import { useLocation } from "react-router-dom";

function Cover() {
    const location = useLocation();
    const { state } = location;
    let { imageURL } = state as { imageURL: string };


    return (
        <>
            <h1>Generated Cover Image (save by right clicking):</h1>
            <div
                className="
                d-flex
                justify-content-center
                align-items-center
                text-center
                mx-auto
                
                rounded-lg
                bg-opacity-25
                bg-slate-100
            "
            >
                <img src={imageURL} alt="Abstract generated cover" />
            </div>
        </>
    );
}

export default Cover;
